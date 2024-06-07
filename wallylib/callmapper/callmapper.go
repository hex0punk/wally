package callmapper

import (
	"container/list"
	"fmt"
	"golang.org/x/tools/go/callgraph"
	"golang.org/x/tools/go/ssa"
	"strings"
	"wally/match"
	"wally/wallylib"
)

type SearchAlgorithm int

const (
	Bfs SearchAlgorithm = iota
	Dfs
)

type CallMapper struct {
	Options Options
	Match   *match.RouteMatch
}

var SearchAlgs = map[string]SearchAlgorithm{
	"bfs": Bfs,
	"dfs": Dfs,
}

type Options struct {
	Filter     string
	MaxFuncs   int
	MaxPaths   int
	PrintNodes bool
	SearchAlg  SearchAlgorithm
}

func NewCallMapper(match *match.RouteMatch, options Options) *CallMapper {
	return &CallMapper{
		Options: options,
		Match:   match,
	}
}

func (cm *CallMapper) AllPathsBFS(s *callgraph.Node, options Options) *match.CallPaths {
	basePos := wallylib.GetFormattedPos(s.Func.Package(), s.Func.Pos())

	initialPath := []string{
		cm.Match.Pos.String(),
		fmt.Sprintf("[%s] %s", s.Func.Name(), basePos),
	}

	callPaths := &match.CallPaths{}
	cm.BFS(s, initialPath, callPaths, options)

	return callPaths
}

func (cm *CallMapper) AllPathsDFS(s *callgraph.Node, options Options) *match.CallPaths {
	visited := make(map[int]bool)

	basePos := wallylib.GetFormattedPos(s.Func.Package(), s.Func.Pos())

	initialPath := []string{
		cm.Match.Pos.String(),
		fmt.Sprintf("[%s] %s", s.Func.Name(), basePos),
	}

	callPaths := &match.CallPaths{}
	callPaths.Paths = []*match.CallPath{}

	cm.DFS(s, visited, initialPath, callPaths, options, nil)

	return callPaths
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
				if e.Caller != nil && !shouldSkipNode(e, options, newPath) && !visited[e.Caller.ID] {
					cm.DFS(e.Caller, visited, newPath, paths, options, e.Site)
				}
			}
		}
	}
}

func (cm *CallMapper) BFS(start *callgraph.Node, initialPath []string, paths *match.CallPaths, options Options) {
	type BFSNode struct {
		Node *callgraph.Node
		Path []string
	}

	queue := list.New()
	queue.PushBack(BFSNode{Node: start, Path: initialPath})

	for queue.Len() > 0 && (options.MaxPaths == 0 || len(paths.Paths) < options.MaxPaths) {
		bfsNodeElm := queue.Front()
		queue.Remove(bfsNodeElm)

		current := bfsNodeElm.Value.(BFSNode)
		currentNode := current.Node
		currentPath := current.Path

		newPath := appendPath(currentNode, currentPath, options, nil)
		mustStop := options.MaxFuncs > 0 && len(newPath) >= options.MaxFuncs

		if len(currentNode.In) == 0 || mustStop {
			paths.InsertPaths(newPath, mustStop)
			if options.MaxPaths > 0 && len(paths.Paths) >= options.MaxPaths {
				break
			}
			continue
		}

		for _, e := range currentNode.In {
			if !shouldSkipNode(e, options, newPath) && !callerInPath(e, newPath) {
				newPathCopy := make([]string, len(newPath))
				copy(newPathCopy, newPath)
				newPathWithCaller := appendPath(e.Caller, newPathCopy, options, e.Site)
				queue.PushBack(BFSNode{Node: e.Caller, Path: newPathWithCaller})
			}
		}
	}
}

func shouldSkipNode(e *callgraph.Edge, options Options, paths []string) bool {
	if options.Filter != "" && e.Caller != nil && !passesFilter(e.Caller, options.Filter) {
		return true
	}
	return false
}

func callerInPath(e *callgraph.Edge, paths []string) bool {
	for _, p := range paths {
		if strings.Contains(p, fmt.Sprintf("[%s]", e.Caller.Func.Name())) {
			return true
		}
	}
	return false
}

func appendPath(s *callgraph.Node, path []string, options Options, site ssa.CallInstruction) []string {
	if site != nil {
		if options.PrintNodes || s.Func.Package() == nil {
			return append(path, s.String())
		}
		fp := wallylib.GetFormattedPos(s.Func.Package(), site.Pos())
		if s.Func.Recover != nil {
			return append(path, fmt.Sprintf("[%s] (recoverable) %s", s.Func.Name(), fp))
		}
		return append(path, fmt.Sprintf("[%s] %s", s.Func.Name(), fp))
	} else {
		return path
		//return append(path, fmt.Sprintf("[%s] %s", s.Func.Name(), wallylib.GetFormattedPos(s.Func.Package(), s.Func.Pos())))
	}
}

func passesFilter(node *callgraph.Node, filter string) bool {
	if node.Func != nil && node.Func.Pkg != nil {
		return strings.HasPrefix(node.Func.Pkg.Pkg.Path(), filter)
	}
	return false
}
