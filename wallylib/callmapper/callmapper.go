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
		//cm.Match.Pos.String(),
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
		//cm.Match.Pos.String(),
		fmt.Sprintf("[%s] %s", s.Func.Name(), basePos),
	}

	callPaths := &match.CallPaths{}
	callPaths.Paths = []*match.CallPath{}

	cm.DFS(s, visited, initialPath, callPaths, options, nil)

	return callPaths
}

func (cm *CallMapper) DFS(destination *callgraph.Node, visited map[int]bool, path []string, paths *match.CallPaths, options Options, site ssa.CallInstruction) {
	newPath := appendNodeToPath(destination, path, options, site)

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
				if e.Caller != nil && !shouldSkipNode(e, options) && !visited[e.Caller.ID] {
					cm.DFS(e.Caller, visited, newPath, paths, options, e.Site)
				}
			}
		}
	}
}

type BFSNode struct {
	ID   int
	Node *callgraph.Node
	Path []string
}

func (cm *CallMapper) BFS(start *callgraph.Node, initialPath []string, paths *match.CallPaths, options Options) {
	queue := list.New()
	queue.PushBack(BFSNode{ID: start.ID, Node: start, Path: initialPath})

	//currId := 0
	for queue.Len() > 0 && (options.MaxPaths == 0 || len(paths.Paths) < options.MaxPaths) {
		//if queue.Len()+len(paths.Paths) >= options.MaxPaths {
		//	break
		//}
		//fmt.Println("one")
		//printQueue(queue)
		// we process the first node - on first iteration, it'd be [Normalize] cogs/cogs.go:156:19 --->
		bfsNodeElm := queue.Front()
		// We remove last elm so we can put it in the front after updating it with new paths
		queue.Remove(bfsNodeElm)

		current := bfsNodeElm.Value.(BFSNode)
		//currentID := current.ID
		currentNode := current.Node
		currentPath := current.Path

		// Are we out of nodes for this currentNode, or have we reached the limit of funcs in a path?
		if len(currentNode.In) == 0 || len(currentPath) >= options.MaxFuncs {
			paths.InsertPaths(currentPath, false)
			continue
		}

		newPath := appendNodeToPath(currentNode, currentPath, options, nil)

		//fmt.Println("two")
		//printQueue(queue)
		// Turns out there ARE nodes that call the currentNode
		//beforeLen := len(newPath)
		allOutsideModule := true
		for _, e := range currentNode.In {
			if callerInPath(e, newPath) {
				continue
			}
			// Do we care about this node, or is it in the path already (if it calls itself)?
			if !shouldSkipNode(e, options) {
				// We care. So let's create a copy of the path. On first iteration this has only our two intial nodes
				newPathCopy := make([]string, len(newPath))
				copy(newPathCopy, newPath)

				// We want to process the new node we added to the path.
				newPathWithCaller := appendNodeToPath(e.Caller, newPathCopy, options, e.Site)
				queue.PushBack(BFSNode{Node: e.Caller, Path: newPathWithCaller})
				allOutsideModule = false
			} else {
				// TODO: the problem here is that we need a way to find out
				// whether any paths were added at all if the node was skipped AND nodes were pushed that require
				// us to keep looking

				// but keep checking the other INs nodes (if there are more) in case there are more nodes we do care about
				continue
			}

			//fmt.Println("three")
			//printQueue(queue)
			if queue.Len()+len(paths.Paths) >= options.MaxPaths {
				break
			}
		}
		if allOutsideModule {
			paths.InsertPaths(currentPath, false)
		}
	}
	for e := queue.Front(); e != nil; e = e.Next() {
		fmt.Println("adding")
		bfsNode := e.Value.(BFSNode)
		paths.InsertPaths(bfsNode.Path, false)
	}
}

// Function to print all elements in the queue
func printQueue(queue *list.List) {
	fmt.Println()
	fmt.Println()
	fmt.Println("Current Queue:")
	for e := queue.Front(); e != nil; e = e.Next() {
		bfsNode := e.Value.(BFSNode)
		fmt.Printf("Node: %s, Path: %v\n", bfsNode.Node.Func.Name(), bfsNode.Path)
	}
	fmt.Println("End of Queue")
	fmt.Println()
}

func shouldSkipNode(e *callgraph.Edge, options Options) bool {
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

func appendNodeToPath(s *callgraph.Node, path []string, options Options, site ssa.CallInstruction) []string {
	if site != nil {
		if options.PrintNodes || s.Func.Package() == nil {
			return append(path, s.String())
		}
		fp := wallylib.GetFormattedPos(s.Func.Package(), site.Pos())
		if fp == "services/webber/main/webber/webber.go:372:20" {
			from := ""
			to := ""
			for idx, ins := range s.Func.Recover.Instrs {
				recFp := wallylib.GetFormattedPos(s.Func.Package(), ins.Pos())
				if idx == 0 {
					from = recFp
				}
				if idx == len(s.Func.Recover.Instrs)-1 {
					to = recFp
				}
			}
			fmt.Println(from, to)
		}
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
