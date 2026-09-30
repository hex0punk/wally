package live

import (
	"go/types"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/hex0punk/wally/navigator"
	"github.com/hex0punk/wally/wallylib"
	"golang.org/x/tools/go/ssa"
	"golang.org/x/tools/go/ssa/ssautil"
)

type funcEntry struct {
	Line int
	Fn   *ssa.Function
}

// searchEntry is one independently-queryable function, in the same
// (package, function, receiver type) shape a query form uses, plus its own
// declaration position so a search result can also jump the code pane
// straight to it (see live/server.go's handleFunctions).
type searchEntry struct {
	Pkg      string
	Function string
	RecvType string
	File     string
	Line     int
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
	cwd      string
	byFile   map[string][]funcEntry // each slice sorted by Line ascending
	search   []searchEntry          // sorted by Function, then Pkg, then RecvType -- see Search
	packages []string               // every distinct Pkg among search entries, sorted -- see Packages
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
	// Same module-boundary narrowing as SourceIndex.ListFiles (see
	// projectFileSet's own doc comment) -- ssautil.AllFunctions covers
	// everything type-checking touched, stdlib and third-party dependencies
	// included, which floods a name search with runtime/stdlib noise a user
	// searching their own codebase never wants. byFile (Resolve's index)
	// stays unfiltered on purpose: a right-clicked line can still legitimately
	// land in third-party code.
	projectFiles := projectFileSet(nav.Packages)

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
		// The same coin-flip applies to any OTHER function with no package
		// of its own that isn't a closure -- confirmed against a real,
		// large codebase: a type embedding another type's pointer gets a
		// synthetic method-promotion thunk for each promoted method (e.g.
		// embedding *Base, which declares Handle, synthesizes a wrapper
		// Handle on the embedding type too -- see sampleapp/embed),
		// positioned at the exact same line as the real method since it has
		// no declaration of its own to be positioned at. Resolve's own final
		// check already rejects fn.Pkg == nil unconditionally, so a
		// tie-broken pick of one of these is never a useful answer either --
		// unlike a closure, which legitimately needs to stay indexed here
		// for Resolve's walk-up-via-Parent() to find when a line lands
		// inside one.
		if !wallylib.IsClosure(fn) && fn.Pkg == nil {
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

		// A closure has no package of its own and isn't independently
		// queryable via --pkg/--func (same reasons Resolve walks past one
		// via Parent()); it's indexed above for Resolve's file:line lookup,
		// but excluded here since a search result must be something a query
		// form can actually target. Third-party/stdlib files are excluded
		// from search for the same reason projectFileSet exists at all --
		// see the note above this loop.
		if wallylib.IsClosure(fn) || fn.Pkg == nil || !projectFiles[file] {
			continue
		}
		recvType := ""
		if recv := fn.Signature.Recv(); recv != nil {
			recvType = receiverTypeName(recv.Type())
		}
		display := file
		if rel, err := filepath.Rel(idx.cwd, file); err == nil {
			display = rel
		}
		idx.search = append(idx.search, searchEntry{
			Pkg:      fn.Pkg.Pkg.Path(),
			Function: fn.Name(),
			RecvType: recvType,
			File:     display,
			Line:     pos.Line,
		})
	}
	for file := range idx.byFile {
		entries := idx.byFile[file]
		sort.Slice(entries, func(i, j int) bool { return entries[i].Line < entries[j].Line })
		idx.byFile[file] = entries
	}
	sort.Slice(idx.search, func(i, j int) bool {
		a, b := idx.search[i], idx.search[j]
		if a.Function != b.Function {
			return a.Function < b.Function
		}
		if a.Pkg != b.Pkg {
			return a.Pkg < b.Pkg
		}
		return a.RecvType < b.RecvType
	})

	pkgSet := map[string]bool{}
	for _, e := range idx.search {
		pkgSet[e.Pkg] = true
	}
	idx.packages = make([]string, 0, len(pkgSet))
	for p := range pkgSet {
		idx.packages = append(idx.packages, p)
	}
	sort.Strings(idx.packages)

	return idx
}

// Packages returns every distinct package that has at least one
// independently-queryable function in the search index (the same set
// Search already draws from), sorted -- the Functions tab's package-filter
// dropdown lists exactly this.
func (idx *FunctionIndex) Packages() []string {
	return idx.packages
}

// Search returns every independently-queryable function whose name --
// or, for a method, "RecvType.Name" -- contains q as a case-insensitive
// substring, in a stable alphabetical-by-function-name order, capped at
// limit. An empty q or non-positive limit returns nil rather than the
// (potentially huge -- tens of thousands of functions in a large
// codebase) whole index; the Files tab's browse list can afford to ship
// everything up front, this can't.
//
// pkgs, when non-empty, restricts results to exactly those packages (the
// Functions tab's package-filter dropdown) -- an empty pkgs means no
// filter at all, matching "nothing checked" reading as "search everything"
// rather than "search nothing."
func (idx *FunctionIndex) Search(q string, limit int, pkgs []string) []searchEntry {
	if q == "" || limit <= 0 {
		return nil
	}
	var pkgFilter map[string]bool
	if len(pkgs) > 0 {
		pkgFilter = make(map[string]bool, len(pkgs))
		for _, p := range pkgs {
			pkgFilter[p] = true
		}
	}
	needle := strings.ToLower(q)
	out := make([]searchEntry, 0, limit)
	for _, e := range idx.search {
		if pkgFilter != nil && !pkgFilter[e.Pkg] {
			continue
		}
		haystack := strings.ToLower(e.Function)
		if e.RecvType != "" {
			haystack = strings.ToLower(e.RecvType) + "." + haystack
		}
		if !strings.Contains(haystack, needle) {
			continue
		}
		out = append(out, e)
		if len(out) >= limit {
			break
		}
	}
	return out
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
