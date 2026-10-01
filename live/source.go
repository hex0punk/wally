package live

import (
	"go/token"
	"os"
	"path/filepath"
	"sort"

	"github.com/hex0punk/wally/navigator"
	"golang.org/x/tools/go/packages"
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
	// projectFiles is the subset of files belonging to a package in the
	// same module as one of --paths' own roots -- see projectFileSet's
	// doc comment for why this exists as a second, narrower set rather
	// than filtering files down directly.
	projectFiles map[string]bool
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
	idx.projectFiles = projectFileSet(nav.Packages)
	return idx
}

// projectFileSet mirrors navigator.expandToModuleClosure's own
// module-boundary rule (see its doc comment: wally's targets are
// essentially always first-party code) at file granularity, for the
// Files tab's browse list specifically.
//
// The allowlist Resolve checks against is deliberately broader than
// this: SourceIndex.files (built from the whole Fset) includes every
// package type-checking touched, stdlib and third-party dependencies
// included, and Resolve should stay that broad -- a graph node reached
// via a real query (e.g. the Invoke+match-filter technique, which
// specifically targets third-party gRPC client code) can legitimately
// have a position in one of those files, and that should still be
// viewable. Browsing the whole transitively-parsed universe, though, is
// mostly noise: a real project has stdlib and dependency source mixed in
// at a 10:1 ratio or worse against its own code. This set narrows only
// what gets listed to browse, not what's servable.
func projectFileSet(roots []*packages.Package) map[string]bool {
	rootModules := map[string]bool{}
	for _, p := range roots {
		if p.Module != nil {
			rootModules[p.Module.Path] = true
		}
	}

	files := map[string]bool{}
	seen := map[*packages.Package]bool{}
	var visit func(p *packages.Package)
	visit = func(p *packages.Package) {
		if p == nil || seen[p] {
			return
		}
		seen[p] = true
		if p.Module == nil || !rootModules[p.Module.Path] {
			return
		}
		for _, f := range p.CompiledGoFiles {
			files[filepath.Clean(f)] = true
		}
		for _, imp := range p.Imports {
			visit(imp)
		}
	}
	for _, p := range roots {
		visit(p)
	}
	return files
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
	abs := resolveAgainstCwd(idx.cwd, file)
	if !idx.files[abs] {
		return "", false
	}
	return abs, true
}

// ListFiles returns first-party files only (same module as one of
// --paths' own roots -- see projectFileSet), relative to the captured cwd
// when possible (matching how positions already read elsewhere in the
// UI), sorted for a stable display order. This is narrower than what
// Resolve will actually serve -- see projectFileSet's doc comment.
func (idx *SourceIndex) ListFiles() []string {
	out := make([]string, 0, len(idx.projectFiles))
	for abs := range idx.projectFiles {
		display := abs
		if rel, err := filepath.Rel(idx.cwd, abs); err == nil {
			display = rel
		}
		out = append(out, display)
	}
	sort.Strings(out)
	return out
}

// resolveAgainstCwd joins a possibly-relative path against cwd (leaving an
// already-absolute one as-is) and cleans the result. Shared by SourceIndex
// and FunctionIndex, which both resolve the same two position shapes
// wally's formatters produce -- relative-to-process-cwd or absolute (see
// graph.go's parsePosition/splitPos doc comment).
func resolveAgainstCwd(cwd, file string) string {
	abs := file
	if !filepath.IsAbs(abs) {
		abs = filepath.Join(cwd, file)
	}
	return filepath.Clean(abs)
}
