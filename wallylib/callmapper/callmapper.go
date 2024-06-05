package callmapper

import (
	"fmt"
	"golang.org/x/tools/go/callgraph"
	"golang.org/x/tools/go/ssa"
	"strings"
	"wally/match"
	"wally/wallylib"
)

type CallMapper struct {
	Options Options
	Match   *match.RouteMatch
}

type Options struct {
	Filter     string
	MaxFuncs   int
	MaxPaths   int
	PrintNodes bool
}

func NewCallMapper(match *match.RouteMatch, options Options) *CallMapper {
	return &CallMapper{
		Options: options,
		Match:   match,
	}
}

func (cm *CallMapper) AllPaths(s *callgraph.Node, options Options) *match.CallPaths {
	visited := make(map[int]bool)
	path := []string{}

	basePos := wallylib.GetFormattedPos(s.Func.Package(), s.Func.Pos())
	path = append(path, fmt.Sprintf("[%s] %s", s.Func.Name(), basePos))

	callPaths := match.CallPaths{}
	callPaths.Paths = []*match.CallPath{}

	cm.DFS(s, visited, path, &callPaths, options, nil)

	return &callPaths
}

func (cm *CallMapper) DFS(destination *callgraph.Node, visited map[int]bool, path []string, paths *match.CallPaths, options Options, site ssa.CallInstruction) {
	newPath := appendPath(destination, path, options, site)

	mustStop := options.MaxFuncs > 0 && len(newPath) >= options.MaxFuncs
	if len(destination.In) == 0 || mustStop {
		paths.InsertPaths(newPath, mustStop)
		return
	}

	// Avoids recursion within a single callpath
	if visited[destination.ID] {
		paths.InsertPaths(newPath, false)
		return
	}
	visited[destination.ID] = true

	defer delete(visited, destination.ID)

	for _, e := range destination.In {
		//if options.MaxFuncs > 0 && (len(newPath) > options.MaxFuncs) {
		//	continue
		//}
		if paths.Paths != nil && options.MaxPaths > 0 && len(paths.Paths) >= options.MaxPaths {
			cm.Match.SSA.PathLimited = true
			continue
		}
		if strings.HasSuffix(e.Caller.Func.Name(), "$bound") {
			paths.InsertPaths(newPath, false)
			return
		} else {
			if !visited[e.Caller.ID] {
				if e.Caller != nil && !cm.shouldSkipNode(e, options, newPath) && !visited[e.Caller.ID] {
					cm.DFS(e.Caller, visited, newPath, paths, options, e.Site)
				}
			}
		}
	}
}

func (cm *CallMapper) shouldSkipNode(e *callgraph.Edge, options Options, paths []string) bool {
	if options.Filter != "" && e.Caller != nil && !passesFilter(e.Caller, options.Filter) {
		return true
	}
	return false
}

func appendPath(s *callgraph.Node, path []string, options Options, site ssa.CallInstruction) []string {
	if site != nil {
		if options.PrintNodes || s.Func.Package() == nil {
			return append(path, s.String())
		}
		fp := wallylib.GetFormattedPos(s.Func.Package(), site.Pos())
		return append(path, fmt.Sprintf("[%s] %s", s.Func.Name(), fp))
	} else {
		return path
	}
}

func passesFilter(node *callgraph.Node, filter string) bool {
	if node.Func != nil && node.Func.Pkg != nil {
		return strings.HasPrefix(node.Func.Pkg.Pkg.Path(), filter)
	}
	return false
}
