package live

import (
	"go/token"
	"os"
	"path/filepath"

	"github.com/hex0punk/wally/navigator"
)

// SourceIndex is an allowlist of every absolute file path wally's own
// analysis actually parsed, built once from the resident Navigator's
// shared token.FileSet -- the same FileSet every position string in the
// system was computed from (see graph.go's parsePosition/splitPos doc
// comments on why a node's embedded position can be either
// relative-to-process-cwd or absolute, depending which of wally's two
// position formatters produced it).
//
// A source request is only ever served if its resolved path is a member
// of this set. That's correct by construction -- it can't miss a file
// wally itself referenced while building the callgraph, since every
// position came from parsing exactly these files -- and it can't be
// tricked by a path-traversal payload, since resolution never trusts a
// request path further than "is it in the set already computed from
// wally's own analysis," never "does cleaning this path make it look
// safe."
type SourceIndex struct {
	cwd   string
	files map[string]bool
}

// NewSourceIndex captures the process's current working directory -- the
// same one every relative position string was formatted against, see
// wallylib.GetFormattedPos -- and walks nav's SSA FileSet once to build
// the allowlist. Call this once, after nav.Build, before serving queries.
func NewSourceIndex(nav *navigator.Navigator) *SourceIndex {
	idx := &SourceIndex{files: map[string]bool{}}
	idx.cwd, _ = os.Getwd()

	if nav.SSA == nil || nav.SSA.Program == nil || nav.SSA.Program.Fset == nil {
		return idx
	}
	nav.SSA.Program.Fset.Iterate(func(f *token.File) bool {
		idx.files[filepath.Clean(f.Name())] = true
		return true
	})
	return idx
}

// Resolve turns a node's embedded position (NodeData.File -- relative to
// the captured cwd, or already absolute) into a verified absolute path.
// ok is false if it isn't a file wally's own analysis actually parsed;
// no filesystem access is attempted in that case, which is the actual
// security boundary, not a check performed after opening the file.
func (idx *SourceIndex) Resolve(file string) (absPath string, ok bool) {
	if file == "" {
		return "", false
	}
	abs := file
	if !filepath.IsAbs(abs) {
		abs = filepath.Join(idx.cwd, file)
	}
	abs = filepath.Clean(abs)
	if !idx.files[abs] {
		return "", false
	}
	return abs, true
}
