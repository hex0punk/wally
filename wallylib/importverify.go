package wallylib

import "golang.org/x/tools/go/packages"

// PackageImportsTransitively reports whether the package at fromPath
// transitively imports the package at toPath, by walking the import graph
// rooted at roots (typically navigator.Navigator.Packages, i.e. every
// package wally's SSA build covers).
//
// This exists to sanity-check a callgraph-derived call path: cha and vta
// both resolve a call through a widely-implemented interface (the leading
// example being grpc.ClientConnInterface.Invoke, which every generated gRPC
// client satisfies) by considering ANY type satisfying that interface a
// possible dispatch target, not just the one actually reachable from a given
// call site. In a codebase where many services share that interface, this
// produces call paths between packages that share no import relationship at
// all -- confirmed in practice tracing a single target's Invoke call across
// ~15 unrelated services, several of which had zero literal reference to the
// target package anywhere in their source, under both cha and vta.
//
// Returns true (i.e. "can't rule it out, don't flag it") when either path
// isn't found in roots at all -- this check is a best-effort sanity filter
// on top of the callgraph, not a replacement for it, and should fail open
// rather than risk flagging legitimate results it can't fully resolve (e.g.
// a path crossing into the standard library or a module wally wasn't told
// to load).
func PackageImportsTransitively(roots []*packages.Package, fromPath, toPath string) bool {
	if fromPath == "" || toPath == "" || fromPath == toPath {
		return true
	}

	index := indexPackagesByPath(roots)
	start, ok := index[fromPath]
	if !ok {
		return true
	}
	if _, ok := index[toPath]; !ok {
		return true
	}

	seen := make(map[string]bool)
	var visit func(p *packages.Package) bool
	visit = func(p *packages.Package) bool {
		if p == nil || seen[p.PkgPath] {
			return false
		}
		seen[p.PkgPath] = true
		if p.PkgPath == toPath {
			return true
		}
		for _, imp := range p.Imports {
			if visit(imp) {
				return true
			}
		}
		return false
	}
	return visit(start)
}

func indexPackagesByPath(roots []*packages.Package) map[string]*packages.Package {
	index := make(map[string]*packages.Package)
	seen := make(map[*packages.Package]bool)
	var visit func(p *packages.Package)
	visit = func(p *packages.Package) {
		if p == nil || seen[p] {
			return
		}
		seen[p] = true
		index[p.PkgPath] = p
		for _, imp := range p.Imports {
			visit(imp)
		}
	}
	for _, p := range roots {
		visit(p)
	}
	return index
}
