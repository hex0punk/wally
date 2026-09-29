package live

import (
	"go/types"
	"os"
	"path/filepath"
	"sort"

	"github.com/hex0punk/wally/navigator"
	"github.com/hex0punk/wally/wallylib"
	"golang.org/x/tools/go/ssa"
	"golang.org/x/tools/go/ssa/ssautil"
)

type funcEntry struct {
	Line int
	Fn   *ssa.Function
}

// FunctionIndex resolves a file:line -- typically a right-click in the
// code viewer -- to its enclosing wally-queryable function: the
// (package, function, receiver type) triple that's already the exact
// input shape navigator.QueryParams expects, so a resolved result can
// drive the same query flow used everywhere else in this UI.
//
// Built from the SSA program's own function set (ssautil.AllFunctions)
// rather than the loaded packages' AST/TypesInfo: nav.Packages only
// covers --paths' root packages, not the auto-expanded same-module
// transitive closure SSA is actually built over (see
// navigator.Navigator.NoAutoDeps), so a right-clicked file can easily be
// outside that root set. Every *ssa.Function's own declaration position
// is available regardless of which set it came from, and that's all this
// needs.
type FunctionIndex struct {
	cwd    string
	byFile map[string][]funcEntry // each slice sorted by Line ascending
}

// NewFunctionIndex captures the process cwd (see resolveAgainstCwd) and
// groups every function in nav's built SSA program by its declaration
// file. Call once, after nav.Build, before serving queries.
func NewFunctionIndex(nav *navigator.Navigator) *FunctionIndex {
	idx := &FunctionIndex{byFile: map[string][]funcEntry{}}
	idx.cwd, _ = os.Getwd()

	if nav.SSA == nil || nav.SSA.Program == nil || nav.SSA.Program.Fset == nil {
		return idx
	}
	fset := nav.SSA.Program.Fset

	for fn := range ssautil.AllFunctions(nav.SSA.Program) {
		if fn.Pos() == 0 {
			continue // no declaration position to index
		}
		// A $bound wrapper is never a useful Resolve answer (no package,
		// not independently queryable -- Resolve already walks past one if
		// it's ever reached via Parent()), and it carries the SAME
		// declaration line as the real method it wraps. Left in, that tie
		// is broken by sort.Slice's instability below, so which of the two
		// same-line entries the binary search in Resolve lands on becomes
		// a coin flip across runs -- confirmed by running the build
		// repeatedly and observing the wrapper and the real method swap
		// positions in the sorted slice from one run to the next.
		// Excluding it here removes the tie entirely, rather than trying
		// to out-sort an unstable sort.
		if wallylib.IsBoundFunc(fn) {
			continue
		}
		pos := fset.Position(fn.Pos())
		// Cleaned to match resolveAgainstCwd's own output exactly -- pos.Filename
		// isn't guaranteed to already be in Clean form (e.g. a non-canonical
		// "../" segment from how go/packages resolved it in a larger module
		// graph), and an uncleaned key here silently never matches Resolve's
		// cleaned lookup. Confirmed against a large, real multi-module
		// codebase: this mismatch made every real function invisible to
		// Resolve there, while it happened to go unnoticed against wally's
		// own small sampleapp module, whose positions were already clean.
		file := filepath.Clean(pos.Filename)
		idx.byFile[file] = append(idx.byFile[file], funcEntry{Line: pos.Line, Fn: fn})
	}
	for file := range idx.byFile {
		entries := idx.byFile[file]
		sort.Slice(entries, func(i, j int) bool { return entries[i].Line < entries[j].Line })
		idx.byFile[file] = entries
	}
	return idx
}

// Resolve finds the function enclosing file:line and reports it in the
// same (package, function, receiver type) shape a query form uses. ok is
// false -- not an error -- for a normal "nothing queryable here" outcome:
// the file isn't indexed, line falls before any function in it, or the
// nearest function turns out to be an unqueryable synthetic wrapper with
// no package of its own.
func (idx *FunctionIndex) Resolve(file string, line int) (pkg, function, recvType string, ok bool) {
	entries := idx.byFile[resolveAgainstCwd(idx.cwd, file)]
	if len(entries) == 0 {
		return "", "", "", false
	}

	// Last entry with Line <= line -- the nearest preceding declaration in
	// this file, which for well-formed Go source is always the enclosing
	// one (sibling top-level funcs/methods never overlap; a closure's own
	// entry is more precise, not less, when line falls inside one).
	i := sort.Search(len(entries), func(i int) bool { return entries[i].Line > line })
	if i == 0 {
		return "", "", "", false // line is before the first function in the file
	}
	fn := entries[i-1].Fn

	// A closure isn't independently queryable via --pkg/--func; walk up to
	// what a user actually means by "this function." A bound-value wrapper
	// has no lexical Parent() at all (walking it panics) and no package of
	// its own, so it can't resolve to anything queryable either.
	for wallylib.IsClosure(fn) {
		if wallylib.IsBoundFunc(fn) {
			return "", "", "", false
		}
		fn = fn.Parent()
		if fn == nil {
			return "", "", "", false
		}
	}
	if wallylib.IsBoundFunc(fn) || fn.Pkg == nil {
		return "", "", "", false
	}

	pkg = fn.Pkg.Pkg.Path()
	function = fn.Name()
	if recv := fn.Signature.Recv(); recv != nil {
		recvType = receiverTypeName(recv.Type())
	}
	return pkg, function, recvType, true
}

// receiverTypeName mirrors the --recv-type convention used throughout
// this codebase: the bare named type, receiver pointer-ness stripped.
func receiverTypeName(t types.Type) string {
	if ptr, ok := t.(*types.Pointer); ok {
		t = ptr.Elem()
	}
	if named, ok := t.(*types.Named); ok {
		return named.Obj().Name()
	}
	return ""
}
