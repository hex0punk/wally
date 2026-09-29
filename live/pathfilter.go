package live

import "github.com/hex0punk/wally/match"

// FilterPathsThroughFunction keeps only the call paths of matches that, at
// some hop, pass through the function identified by (pkg, function,
// recvType) -- the "does any path to this sink pass through that source"
// question the right-click "Find path from source here" flow answers.
//
// This needs no new graph traversal: match.CallPath.Nodes is already a
// linear, innermost-first chain of wallynode.WallyNode, and each node's
// Caller.Func is literally "the function that makes this hop's call" (see
// match.CallPath's own doc comment on Nodes ordering). Checking whether
// the source function appears anywhere in that chain is exactly "does
// this path pass through the source."
//
// A match with zero surviving paths is dropped entirely -- a sink can
// have real paths that don't happen to go through the requested source,
// and those aren't what this question is asking about.
func FilterPathsThroughFunction(matches []match.RouteMatch, pkg, function, recvType string) []match.RouteMatch {
	var out []match.RouteMatch
	for _, m := range matches {
		if m.SSA == nil || m.SSA.CallPaths == nil {
			continue
		}

		var kept []*match.CallPath
		for _, p := range m.SSA.CallPaths.Paths {
			if pathPassesThrough(p, pkg, function, recvType) {
				kept = append(kept, p)
			}
		}
		if len(kept) == 0 {
			continue
		}

		filtered := m
		ssaCopy := *m.SSA
		ssaCopy.CallPaths = &match.CallPaths{Paths: kept}
		filtered.SSA = &ssaCopy
		out = append(out, filtered)
	}
	return out
}

func pathPassesThrough(p *match.CallPath, pkg, function, recvType string) bool {
	for _, node := range p.Nodes {
		if node.Caller == nil || node.Caller.Func == nil {
			continue
		}
		fn := node.Caller.Func
		if fn.Pkg == nil || fn.Pkg.Pkg == nil {
			continue
		}
		if fn.Pkg.Pkg.Path() != pkg || fn.Name() != function {
			continue
		}
		fnRecvType := ""
		if recv := fn.Signature.Recv(); recv != nil {
			fnRecvType = receiverTypeName(recv.Type())
		}
		if fnRecvType != recvType {
			continue
		}
		return true
	}
	return false
}
