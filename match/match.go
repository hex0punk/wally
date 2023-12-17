package match

import (
	"go/token"
	"go/types"
	"golang.org/x/tools/go/callgraph"
	"golang.org/x/tools/go/ssa"
	"strings"
	"wally/indicator"
	match "wally/wallylib"
)

type SSAContext struct {
	EnclosedByFunc *ssa.Function
	Edges          []*callgraph.Edge
	CallPaths      [][]string
}

type RouteMatch struct {
	Indicator  indicator.Indicator // It should be FuncInfo instead
	Params     map[string]string
	Pos        token.Position
	Signature  *types.Signature
	EnclosedBy string
	SSA        *SSAContext
}

func NewRouteMatch(indicator indicator.Indicator, pos token.Position) RouteMatch {
	return RouteMatch{
		Indicator: indicator,
		Pos:       pos,
		SSA:       &SSAContext{},
	}
}

func (r *RouteMatch) AllPaths(s *callgraph.Node, filter string, recLimit int) [][]string {
	visited := make(map[*callgraph.Node]bool)
	paths := [][]string{}
	path := []string{}

	r.DFS(s, visited, path, &paths, filter, recLimit)

	// TODO: We have to do this given that the cha callgraph algorithm seems to return duplicate paths at times.
	// I need to test other algorithms available to see if I get better results (without duplicate paths)
	res := match.DedupPaths(paths)

	return res
}

func (r *RouteMatch) DFS(s *callgraph.Node, visited map[*callgraph.Node]bool, path []string, paths *[][]string, filter string, recLimit int) {
	visited[s] = true
	if !strings.HasSuffix(s.String(), "$bound") {
		if s.Func != nil {
			path = append(path, s.String())
		}
	}

	if len(s.In) == 0 {
		*paths = append(*paths, path)
	} else {
		for _, e := range s.In {
			if recLimit > 0 && len(*paths) >= recLimit {
				delete(visited, s)
				*paths = append(*paths, path)
				return
			}
			if filter != "" && e.Caller != nil {
				if !passesFilter(e.Caller, filter) {
					delete(visited, s)
					*paths = append(*paths, path)
					return
				}
			}
			if e.Caller != nil && !visited[e.Caller] {
				r.DFS(e.Caller, visited, path, paths, filter, recLimit)
			}
		}
	}

	delete(visited, s)
	//path = path[:len(path)-1]
}

func passesFilter(node *callgraph.Node, filter string) bool {
	if node.Func != nil && node.Func.Pkg != nil {
		return strings.HasPrefix(node.Func.Pkg.Pkg.Path(), filter)
	}
	return false
}
