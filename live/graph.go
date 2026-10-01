// Package live serves a resident wally query as an HTTP API plus a small
// embedded web UI that renders results as a Cytoscape.js graph -- the same
// "build the SSA/callgraph once, query it many times" pattern as `wally
// shell` (see navigator.Navigator.Query), fronted by a browser instead of a
// stdin loop.
package live

import (
	"fmt"
	"path/filepath"
	"sort"
	"strconv"
	"strings"

	"github.com/hex0punk/wally/match"
	"github.com/hex0punk/wally/wallylib"
	"github.com/hex0punk/wally/wallynode"
)

// Elements is the Cytoscape.js elements() shape: the frontend feeds this
// straight into cy.add(resp.elements).
type Elements struct {
	Nodes []NodeElement `json:"nodes"`
	Edges []EdgeElement `json:"edges"`
}

type NodeElement struct {
	Data    NodeData `json:"data"`
	Classes string   `json:"classes,omitempty"`
}

type NodeData struct {
	ID          string `json:"id"`
	Label       string `json:"label"`
	Short       string `json:"short"`
	Kind        string `json:"kind"` // "frame" | "target"
	Unconfirmed bool   `json:"unconfirmed"`
	Recoverable bool   `json:"recoverable"`
	Truncated   bool   `json:"truncated"`
	Paths       []int  `json:"paths"`
	// File/Line/Col are parsed from Label by parsePosition -- File is
	// exactly the form wally itself embedded (relative-to-process-cwd for
	// a call-path frame, or absolute for some match target positions; see
	// live/source.go's SourceIndex for why the source endpoint doesn't
	// trust this at face value). Line/Col are 0 and File is "" when a
	// label doesn't carry a recognizable position at all (e.g. a raw SSA
	// value's default string form) -- the frontend shows "no source
	// available" for those instead of attempting a request.
	File string `json:"file"`
	Line int    `json:"line"`
	Col  int    `json:"col"`
	// ResolvedArgs carries the target's own matched call site's resolved
	// argument values (see wallylib.ResolveAllArgs) so the code pane can
	// draw a hoverable box on each one. Only ever set on a "target" node --
	// a "frame" node is a callgraph hop with no retained AST/argument info.
	ResolvedArgs []wallylib.ResolvedArg `json:"resolvedArgs,omitempty"`
}

type EdgeElement struct {
	Data    EdgeData `json:"data"`
	Classes string   `json:"classes,omitempty"`
}

type EdgeData struct {
	ID          string `json:"id"`
	Source      string `json:"source"`
	Target      string `json:"target"`
	Unconfirmed bool   `json:"unconfirmed"`
	// NoImport marks specifically the innermost edge (the call directly
	// into the matched target) of a path where CallPath.NoImportPathFound()
	// -- the one hop the CLI's red warning calls out by name ("NO IMPORT
	// PATH FOUND for even the call into the target"). Unlike Unconfirmed,
	// this is not a confirmed-anywhere-wins rollup: any path flagging this
	// edge as the no-import culprit wins, since it's a "look at this
	// specifically" warning, not a confidence average.
	NoImport bool `json:"noImport"`
}

// PathInfo carries the same per-path badges the CLI's pathHeader prints
// above each tree, so the UI can render the same warnings next to the
// graph instead of just coloring it.
type PathInfo struct {
	ID                int      `json:"id"`
	MatchID           string   `json:"matchId"`
	Nodes             []string `json:"nodes"` // outermost -> target, display order
	NodeLimited       bool     `json:"nodeLimited"`
	FilterLimited     bool     `json:"filterLimited"`
	Recoverable       bool     `json:"recoverable"`
	ConfirmedDepth    int      `json:"confirmedDepth"`
	UnconfirmedCount  int      `json:"unconfirmedCount"`
	NoImportPathFound bool     `json:"noImportPathFound"`
}

// MatchInfo is the box-level metadata pretty.go's PrintMach renders --
// listed even for a match that contributes nothing to the graph (e.g. no
// SSA/CallPaths resolved), so the UI can still report "found N matches."
type MatchInfo struct {
	ID          string `json:"id"`
	Package     string `json:"package"`
	Function    string `json:"function"`
	Module      string `json:"module"`
	EnclosedBy  string `json:"enclosedBy"`
	Position    string `json:"position"`
	PathLimited bool   `json:"pathLimited"`
}

// buildNode is graph-building's own bookkeeping for a node -- the exported
// NodeData is derived from this once building is done, so JSON output never
// carries confirmedSeen.
type buildNode struct {
	id            string
	label         string
	kind          string
	recoverable   bool
	truncated     bool
	unconfirmed   bool
	confirmedSeen bool
	paths         map[int]bool
	// resolvedArgs is only ever set on a "target" node -- see NodeData's
	// own doc comment for why a "frame" node never carries this.
	resolvedArgs []wallylib.ResolvedArg
}

type nodeBuilder struct {
	order []string
	byKey map[string]*buildNode
}

func newNodeBuilder() *nodeBuilder {
	return &nodeBuilder{byKey: map[string]*buildNode{}}
}

func (b *nodeBuilder) upsert(label, kind string) *buildNode {
	n, ok := b.byKey[label]
	if !ok {
		n = &buildNode{
			id:          fmt.Sprintf("n%d", len(b.order)),
			label:       label,
			kind:        kind,
			recoverable: wallynode.HasRecoverableMarker(label),
			paths:       map[int]bool{},
		}
		b.byKey[label] = n
		b.order = append(b.order, label)
	}
	return n
}

// markConfirmed folds one path's verdict for this node into its overall
// state: confirmed-anywhere wins, so the node only ends up Unconfirmed if
// every path touching it marked it unconfirmed.
func (n *buildNode) markConfirmed(unconfirmed bool) {
	if n.confirmedSeen {
		return
	}
	if !unconfirmed {
		n.confirmedSeen = true
		n.unconfirmed = false
		return
	}
	n.unconfirmed = true
}

// buildEdge is graph-building's own bookkeeping for an edge, keyed by its
// endpoints' node-string labels (resolved to node IDs at render time, once
// every node has been assigned one).
type buildEdge struct {
	id            string
	source        string // label, not yet a node ID
	target        string // label, not yet a node ID
	unconfirmed   bool
	confirmedSeen bool
	noImport      bool
}

type edgeBuilder struct {
	order []string
	byKey map[string]*buildEdge
}

func newEdgeBuilder() *edgeBuilder {
	return &edgeBuilder{byKey: map[string]*buildEdge{}}
}

func (b *edgeBuilder) upsert(source, target string) *buildEdge {
	key := source + "->" + target
	e, ok := b.byKey[key]
	if !ok {
		e = &buildEdge{id: fmt.Sprintf("e%d", len(b.order)), source: source, target: target}
		b.byKey[key] = e
		b.order = append(b.order, key)
	}
	return e
}

// markConfirmed mirrors buildNode.markConfirmed: confirmed-anywhere wins.
func (e *buildEdge) markConfirmed(unconfirmed bool) {
	if e.confirmedSeen {
		return
	}
	if !unconfirmed {
		e.confirmedSeen = true
		e.unconfirmed = false
		return
	}
	e.unconfirmed = true
}

// BuildGraph walks matches' call paths exactly like reporter/pretty.go's
// buildPathTree (innermost frame first, growing outward, with
// SSA.TargetPos as the leaf), merges them into one DAG keyed by node
// string, and returns it alongside per-path and per-match metadata for the
// UI's badge lists.
func BuildGraph(matches []match.RouteMatch) (Elements, []PathInfo, []MatchInfo) {
	nodes := newNodeBuilder()
	edges := newEdgeBuilder()
	var pathInfos []PathInfo
	var matchInfos []MatchInfo
	pathID := 0

	for _, m := range matches {
		matchInfos = append(matchInfos, buildMatchInfo(m))

		if m.SSA == nil || m.SSA.CallPaths == nil {
			continue
		}

		for _, p := range m.SSA.CallPaths.Paths {
			id := pathID
			pathID++

			targetKey := m.SSA.TargetPos
			targetNode := nodes.upsert(targetKey, "target")
			targetNode.paths[id] = true
			targetNode.resolvedArgs = m.ResolvedArgs

			displayNodes := make([]string, 0, len(p.Nodes)+1)

			prevKey := targetKey
			for x := 0; x < len(p.Nodes); x++ {
				label := p.Nodes[x].NodeString
				unconfirmed := p.FrameUnconfirmed(x)

				n := nodes.upsert(label, "frame")
				n.markConfirmed(unconfirmed)
				n.paths[id] = true
				if x == len(p.Nodes)-1 && (p.NodeLimited || p.FilterLimited) {
					n.truncated = true
				}

				e := edges.upsert(label, prevKey)
				e.markConfirmed(unconfirmed)
				if x == 0 && p.NoImportPathFound() {
					e.noImport = true
				}

				prevKey = label
			}

			// displayNodes is outermost -> target, matching how the CLI
			// tree reads top (root/outermost) to bottom (target leaf).
			for x := len(p.Nodes) - 1; x >= 0; x-- {
				displayNodes = append(displayNodes, p.Nodes[x].NodeString)
			}
			displayNodes = append(displayNodes, targetKey)

			pathInfos = append(pathInfos, PathInfo{
				ID:                id,
				MatchID:           m.MatchId,
				Nodes:             displayNodes,
				NodeLimited:       p.NodeLimited,
				FilterLimited:     p.FilterLimited,
				Recoverable:       p.Recoverable,
				ConfirmedDepth:    p.ConfirmedDepth,
				UnconfirmedCount:  p.UnconfirmedCount(),
				NoImportPathFound: p.NoImportPathFound(),
			})
		}
	}

	return Elements{
		Nodes: renderNodes(nodes),
		Edges: renderEdges(edges, nodes),
	}, pathInfos, matchInfos
}

func buildMatchInfo(m match.RouteMatch) MatchInfo {
	enclosedBy := m.EnclosedBy
	pathLimited := false
	if m.SSA != nil {
		if m.SSA.EnclosedByFunc != nil {
			enclosedBy = m.SSA.EnclosedByFunc.String()
		}
		pathLimited = m.SSA.PathLimited
	}
	return MatchInfo{
		ID:          m.MatchId,
		Package:     m.Indicator.Package,
		Function:    m.Indicator.Function,
		Module:      m.Module,
		EnclosedBy:  enclosedBy,
		Position:    fmt.Sprintf("%s:%d", m.Pos.Filename, m.Pos.Line),
		PathLimited: pathLimited,
	}
}

func renderNodes(b *nodeBuilder) []NodeElement {
	out := make([]NodeElement, 0, len(b.order))
	for _, key := range b.order {
		n := b.byKey[key]
		pathIDs := make([]int, 0, len(n.paths))
		for id := range n.paths {
			pathIDs = append(pathIDs, id)
		}
		sort.Ints(pathIDs)
		file, line, col, _ := parsePosition(n.label)
		out = append(out, NodeElement{
			Data: NodeData{
				ID:           n.id,
				Label:        n.label,
				Short:        shortLabel(n.label),
				Kind:         n.kind,
				Unconfirmed:  n.unconfirmed,
				Recoverable:  n.recoverable,
				Truncated:    n.truncated,
				Paths:        pathIDs,
				File:         file,
				Line:         line,
				Col:          col,
				ResolvedArgs: n.resolvedArgs,
			},
			Classes: classesFor(n.kind, n.unconfirmed, n.recoverable, n.truncated),
		})
	}
	return out
}

// renderEdges resolves each edge's node-string endpoints (used as the
// dedup key while building) to their assigned graph node IDs.
func renderEdges(b *edgeBuilder, nodes *nodeBuilder) []EdgeElement {
	out := make([]EdgeElement, 0, len(b.order))
	for _, key := range b.order {
		e := b.byKey[key]
		classes := ""
		if e.noImport {
			classes = "no-import"
		} else if e.unconfirmed {
			classes = "unconfirmed"
		}
		out = append(out, EdgeElement{
			Data: EdgeData{
				ID:          e.id,
				Source:      nodes.byKey[e.source].id,
				Target:      nodes.byKey[e.target].id,
				Unconfirmed: e.unconfirmed,
				NoImport:    e.noImport,
			},
			Classes: classes,
		})
	}
	return out
}

// shortLabel trims a node's full position string down to the bracketed
// function name plus a file:line (e.g. "pkg.[Foo] path/to/file.go:12:3" ->
// "pkg.[Foo]\nfile.go:12:3") for the graph's on-canvas label; the full
// string stays available as Label for a details panel.
//
// Two distinct call sites can share the exact same "pkg.[Func]" text -- a
// helper with that name defined once but reached from two different call
// sites, or two different files/wrappers that happen to share a package and
// function name -- and nodes are deduplicated by the FULL label, not this
// short one, so two on-canvas boxes with identical short text are always
// genuinely different sites, never a dedup bug. Dropping the position
// entirely (as an earlier version of this did) made that indistinguishable
// at a glance; keeping the file:line is the minimum needed to tell them
// apart without clicking into the details panel for every node.
func shortLabel(label string) string {
	i := strings.Index(label, "] ")
	if i == -1 {
		return label
	}
	head := label[:i+1]
	file, line, col, ok := parsePosition(label)
	if !ok {
		return head
	}
	base := filepath.Base(file)
	if col > 0 {
		return fmt.Sprintf("%s\n%s:%d:%d", head, base, line, col)
	}
	return fmt.Sprintf("%s\n%s:%d", head, base, line)
}

// parsePosition extracts the trailing position from a wally node label --
// the last whitespace-separated field, whether or not a "(recoverable)"
// marker sits between the "]" and it. Reports ok=false for a label with no
// recognizable position at all (e.g. a raw SSA value's default String()
// form, which has no "] " to anchor on).
func parsePosition(label string) (file string, line, col int, ok bool) {
	i := strings.Index(label, "] ")
	if i == -1 {
		return "", 0, 0, false
	}
	fields := strings.Fields(label[i+2:])
	if len(fields) == 0 {
		return "", 0, 0, false
	}
	return splitPos(fields[len(fields)-1])
}

// splitPos parses a "file:line:col" or "file:line" position string from
// the right -- both are real formats wally itself produces (see
// wallylib.GetFormattedPos, relative-to-cwd, vs token.Position.String(),
// absolute, used in different branches of callmapper.go's initPath) -- so
// this can't assume a fixed number of colon-separated parts. The file
// portion is joined back from whatever's left, which tolerates a Windows
// drive-letter colon even though that's not a path this tool expects to
// see in practice.
func splitPos(pos string) (file string, line, col int, ok bool) {
	parts := strings.Split(pos, ":")
	if len(parts) < 2 {
		return "", 0, 0, false
	}
	if len(parts) >= 3 {
		if l, err := strconv.Atoi(parts[len(parts)-2]); err == nil {
			if c, err := strconv.Atoi(parts[len(parts)-1]); err == nil {
				if f := strings.Join(parts[:len(parts)-2], ":"); f != "" {
					return f, l, c, true
				}
			}
		}
	}
	if l, err := strconv.Atoi(parts[len(parts)-1]); err == nil {
		if f := strings.Join(parts[:len(parts)-1], ":"); f != "" {
			return f, l, 0, true
		}
	}
	return "", 0, 0, false
}

func classesFor(kind string, unconfirmed, recoverable, truncated bool) string {
	var classes []string
	classes = append(classes, kind)
	if unconfirmed {
		classes = append(classes, "unconfirmed")
	}
	if recoverable {
		classes = append(classes, "recoverable")
	}
	if truncated {
		classes = append(classes, "truncated")
	}
	return strings.Join(classes, " ")
}
