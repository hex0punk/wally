package wallylib

import (
	"go/types"

	"golang.org/x/tools/go/packages"
)

// PackageIndex is a flattened, by-path lookup over a package import graph,
// built once (see NewPackageIndex) and queried cheaply many times -- e.g.
// once per hop of every call path found in a session, which would be far
// too expensive to rebuild the underlying walk for on every call.
type PackageIndex struct {
	byPath map[string]*packages.Package
}

// NewPackageIndex flattens the import graph rooted at roots (typically
// navigator.Navigator.Packages, i.e. every package wally's SSA build
// covers) into a single path -> package lookup.
func NewPackageIndex(roots []*packages.Package) *PackageIndex {
	byPath := make(map[string]*packages.Package)
	seen := make(map[*packages.Package]bool)
	var visit func(p *packages.Package)
	visit = func(p *packages.Package) {
		if p == nil || seen[p] {
			return
		}
		seen[p] = true
		byPath[p.PkgPath] = p
		for _, imp := range p.Imports {
			visit(imp)
		}
	}
	for _, p := range roots {
		visit(p)
	}
	return &PackageIndex{byPath: byPath}
}

// DirectlyImports reports whether the package at fromPath imports the
// package at toPath directly (one hop) -- i.e. whether a literal function
// call from a fromPath function to a toPath function is even syntactically
// possible, absent same-package calls. Returns true (i.e. "can't rule it
// out") if either package isn't in the index, so this fails open rather
// than flagging something it can't fully resolve.
//
// This is the primitive verifyCallPathImports (navigator.go) uses to check
// each hop of a callgraph-derived path individually -- see its doc comment
// for why a single aggregate "is there some transitive path" check isn't
// precise enough to both catch a fully-fabricated call (cha/vta resolving a
// widely-implemented interface method to an unrelated implementation) and
// avoid flagging a real multi-hop chain that happens to have generic
// framework/bootstrap noise prepended ahead of its real entry point.
func (idx *PackageIndex) DirectlyImports(fromPath, toPath string) bool {
	if fromPath == "" || toPath == "" || fromPath == toPath {
		return true
	}
	from, ok := idx.byPath[fromPath]
	if !ok {
		return true
	}
	if _, ok := idx.byPath[toPath]; !ok {
		return true
	}
	for _, imp := range from.Imports {
		if imp.PkgPath == toPath {
			return true
		}
	}
	return false
}

// ResolveNamedType looks up a package-scope type declaration by its
// package path and unqualified name (e.g. "some/pkg", "SomeType"),
// returning its types.Type. Returns nil if the package isn't in this
// index, or the package declares no such name, or the name isn't a type
// -- any of which fail-open the same way DirectlyImports does, since this
// is a best-effort lookup over an already-loaded program, not a guarantee
// the target actually exists as specified.
func (idx *PackageIndex) ResolveNamedType(pkgPath, typeName string) types.Type {
	pkg, ok := idx.byPath[pkgPath]
	if !ok || pkg.Types == nil {
		return nil
	}
	obj := pkg.Types.Scope().Lookup(typeName)
	if obj == nil {
		return nil
	}
	if _, ok := obj.(*types.TypeName); !ok {
		return nil
	}
	return obj.Type()
}
