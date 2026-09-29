package live

import (
	"embed"
	"encoding/json"
	"fmt"
	"io/fs"
	"log"
	"net/http"
	"os"
	"strconv"
	"sync"
	"time"

	"github.com/hex0punk/wally/navigator"
)

//go:embed web
var webFS embed.FS

// Info is the static, build-time metadata /api/info returns for the UI's
// header line -- distinct from server/server.go's own embed, this never
// touches it.
type Info struct {
	Paths        []string `json:"paths"`
	CallgraphAlg string   `json:"callgraphAlg"`
	BuildTime    string   `json:"buildTime"`
	PackageCount int      `json:"packageCount"`
}

// Server answers wally queries over HTTP against an already-built
// Navigator, and serves the embedded web UI at "/".
//
// nav.Query is not safe for concurrent use (see navigator/query.go), so
// every query -- match-finding and graph-building both -- runs under mu.
// Queries are sub-second and this is a single-user local tool, so a mutex
// is enough for now; see the "wally live" design notes for why per-request
// Navigator copies are deferred rather than attempted here.
type Server struct {
	mu            sync.Mutex
	nav           *navigator.Navigator
	info          Info
	sourceIndex   *SourceIndex
	functionIndex *FunctionIndex
}

func NewServer(nav *navigator.Navigator, info Info) *Server {
	return &Server{
		nav:           nav,
		info:          info,
		sourceIndex:   NewSourceIndex(nav),
		functionIndex: NewFunctionIndex(nav),
	}
}

func (s *Server) Handler() (http.Handler, error) {
	sub, err := fs.Sub(webFS, "web")
	if err != nil {
		return nil, fmt.Errorf("embedding web UI: %w", err)
	}

	mux := http.NewServeMux()
	mux.Handle("/", http.FileServer(http.FS(sub)))
	mux.HandleFunc("/api/info", s.handleInfo)
	mux.HandleFunc("/api/query", s.handleQuery)
	mux.HandleFunc("/api/source", s.handleSource)
	mux.HandleFunc("/api/files", s.handleFiles)
	mux.HandleFunc("/api/enclosing", s.handleEnclosing)
	return mux, nil
}

// ListenAndServe starts the server on addr. It never returns on success.
func (s *Server) ListenAndServe(addr string) error {
	handler, err := s.Handler()
	if err != nil {
		return err
	}
	srv := &http.Server{
		Addr:              addr,
		Handler:           handler,
		ReadHeaderTimeout: 5 * time.Second,
	}
	return srv.ListenAndServe()
}

func (s *Server) handleInfo(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, s.info)
}

type queryResponse struct {
	Query      navigator.QueryParams `json:"query"`
	ElapsedMs  float64               `json:"elapsedMs"`
	MatchCount int                   `json:"matchCount"`
	NodeCount  int                   `json:"nodeCount"`
	EdgeCount  int                   `json:"edgeCount"`
	Elements   Elements              `json:"elements"`
	Paths      []PathInfo            `json:"paths"`
	Matches    []MatchInfo           `json:"matches"`
}

func (s *Server) handleQuery(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeError(w, http.StatusMethodNotAllowed, "POST only")
		return
	}

	r.Body = http.MaxBytesReader(w, r.Body, 1<<20) // 1MB, generous for a hand-typed query
	q := navigator.DefaultQueryParams()
	dec := json.NewDecoder(r.Body)
	dec.DisallowUnknownFields()
	if err := dec.Decode(&q); err != nil {
		writeError(w, http.StatusBadRequest, fmt.Sprintf("invalid request body: %s", err))
		return
	}
	if err := q.Validate(); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}

	start := time.Now()
	s.mu.Lock()
	matches := s.nav.Query(q)
	elements, paths, matchInfos := BuildGraph(matches)
	s.mu.Unlock()
	elapsed := time.Since(start)

	log.Printf("query pkg=%s func=%s recv-type=%q matches=%d paths=%d elapsed=%s",
		q.Pkg, q.Func, q.RecvType, len(matches), len(paths), elapsed)

	writeJSON(w, http.StatusOK, queryResponse{
		Query:      q,
		ElapsedMs:  float64(elapsed.Microseconds()) / 1000.0,
		MatchCount: len(matches),
		NodeCount:  len(elements.Nodes),
		EdgeCount:  len(elements.Edges),
		Elements:   elements,
		Paths:      paths,
		Matches:    matchInfos,
	})
}

type sourceResponse struct {
	Path    string `json:"path"`
	Content string `json:"content"`
	Line    int    `json:"line"`
}

// maxSourceBytes backstops against a pathological generated file (some
// .pb.go files run large) -- generous for anything hand-written.
const maxSourceBytes = 5 << 20

// handleSource serves a file's content for the code viewer pane. file
// must be exactly what a prior /api/query response put in a node's
// NodeData.File -- it is never trusted at face value; s.sourceIndex only
// resolves it if it's a file wally's own analysis actually parsed (see
// source.go), which is the real security boundary here, not path
// cleaning.
func (s *Server) handleSource(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeError(w, http.StatusMethodNotAllowed, "GET only")
		return
	}

	file := r.URL.Query().Get("file")
	absPath, ok := s.sourceIndex.Resolve(file)
	if !ok {
		writeError(w, http.StatusNotFound, "not a file wally's analysis parsed")
		return
	}

	stat, err := os.Stat(absPath)
	if err != nil {
		writeError(w, http.StatusNotFound, "file not found")
		return
	}
	if stat.Size() > maxSourceBytes {
		writeError(w, http.StatusRequestEntityTooLarge, "file too large to display")
		return
	}

	content, err := os.ReadFile(absPath)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "could not read file")
		return
	}

	line, _ := strconv.Atoi(r.URL.Query().Get("line")) // 0 if absent/invalid; frontend just won't highlight a line

	writeJSON(w, http.StatusOK, sourceResponse{
		Path:    file,
		Content: string(content),
		Line:    line,
	})
}

type filesResponse struct {
	Files []string `json:"files"`
}

// handleFiles lists first-party files only (see SourceIndex.ListFiles) for
// the Files tab's browse/filter list. This is narrower than what
// /api/source will actually serve -- every file here is guaranteed
// loadable, but a graph node can still resolve to a broader file /api/source
// accepts that isn't listed here (e.g. third-party code reached via a
// match-filter query).
func (s *Server) handleFiles(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeError(w, http.StatusMethodNotAllowed, "GET only")
		return
	}
	writeJSON(w, http.StatusOK, filesResponse{Files: s.sourceIndex.ListFiles()})
}

type enclosingResponse struct {
	Ok       bool   `json:"ok"`
	Pkg      string `json:"pkg"`
	Function string `json:"function"`
	RecvType string `json:"recvType"`
}

// handleEnclosing resolves a file:line -- a right-click in the code
// viewer -- to its enclosing function, in the same shape a query form
// uses. Ok=false is a normal outcome (nothing queryable at that line, e.g.
// an import or a package-level var), not an error, so this is always 200.
func (s *Server) handleEnclosing(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeError(w, http.StatusMethodNotAllowed, "GET only")
		return
	}

	file := r.URL.Query().Get("file")
	line, _ := strconv.Atoi(r.URL.Query().Get("line"))

	pkg, function, recvType, ok := s.functionIndex.Resolve(file, line)
	writeJSON(w, http.StatusOK, enclosingResponse{
		Ok:       ok,
		Pkg:      pkg,
		Function: function,
		RecvType: recvType,
	})
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}

func writeError(w http.ResponseWriter, status int, msg string) {
	writeJSON(w, status, map[string]string{"error": msg})
}
