package callmapper

import (
	"container/list"
	"errors"
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
	Stop    bool
}

var SearchAlgs = map[string]SearchAlgorithm{
	"bfs": Bfs,
	"dfs": Dfs,
}

// TODO: this should be path of the callpath structs in match pkg
type BFSNode struct {
	Node *callgraph.Node
	Path []string
}

type Options struct {
	Filter            string
	MaxFuncs          int
	MaxPaths          int
	ContinueAfterMain bool
	PrintNodes        bool
	SearchAlg         SearchAlgorithm
}

func NewCallMapper(match *match.RouteMatch, options Options) *CallMapper {
	return &CallMapper{
		Options: options,
		Match:   match,
	}
}

func (cm *CallMapper) initPath(s *callgraph.Node) ([]string, string) {
	basePos := wallylib.GetFormattedPos(s.Func.Package(), s.Func.Pos())
	baseStr := fmt.Sprintf("%s.[%s] %s", s.Func.Pkg.Pkg.Name(), s.Func.Name(), basePos)
	if s.Func.Recover != nil {
		rec, err := findDeferRecover(s.Func, s.Func.Recover.Index-1)
		if err != nil {
			baseStr = fmt.Sprintf("%s.[%s] (%s) %s", s.Func.Pkg.Pkg.Name(), s.Func.Name(), err.Error(), basePos)
		}
		if rec {
			baseStr = fmt.Sprintf("%.[%s] (recoverable) %s", s.Func.Pkg.Pkg.Name(), s.Func.Name(), basePos)
		}
	}
	initialPath := []string{
		baseStr,
	}
	return initialPath, baseStr
}

func (cm *CallMapper) AllPathsBFS(s *callgraph.Node, options Options) *match.CallPaths {
	initialPath, _ := cm.initPath(s)
	callPaths := &match.CallPaths{}
	cm.BFS(s, initialPath, callPaths, options)
	return callPaths
}

func (cm *CallMapper) AllPathsDFS(s *callgraph.Node, options Options) *match.CallPaths {
	visited := make(map[int]bool)
	initialPath, _ := cm.initPath(s)
	callPaths := &match.CallPaths{}
	callPaths.Paths = []*match.CallPath{}
	cm.DFS(s, visited, initialPath, callPaths, options, nil)
	return callPaths
}

func (cm *CallMapper) DFS(destination *callgraph.Node, visited map[int]bool, path []string, paths *match.CallPaths, options Options, site ssa.CallInstruction) {
	newPath := appendNodeToPath(destination, path, options, site)
	if (destination.Func.Name() == "main" || destination.Func.Name() == "main$1") && !options.ContinueAfterMain {
		paths.InsertPaths(newPath, false, false)
		cm.Stop = false
		return
	}

	mustStop := options.MaxFuncs > 0 && len(newPath) >= options.MaxFuncs
	if len(destination.In) == 0 || mustStop || cm.Stop {
		paths.InsertPaths(newPath, mustStop, cm.Stop)
		cm.Stop = false
		return
	}

	// Avoids recursion within a single callpath
	if visited[destination.ID] {
		paths.InsertPaths(newPath, false, false)
		return
	}
	visited[destination.ID] = true

	defer delete(visited, destination.ID)

	allOutsideModule := true
	for _, e := range destination.In {
		//if options.MaxFuncs > 0 && (len(newPath) > options.MaxFuncs) {
		//	continue
		//}
		if paths.Paths != nil && options.MaxPaths > 0 && len(paths.Paths) >= options.MaxPaths {
			cm.Match.SSA.PathLimited = true
			continue
		}
		if visited[e.Caller.ID] {
			continue
		}
		if !shouldSkipNode(e, options) {
			allOutsideModule = false
			cm.DFS(e.Caller, visited, newPath, paths, options, e.Site)
		}
	}
	if allOutsideModule {
		// TODO: This is a quick and dirty solution to marking a path as going outside the module
		// This should be handled diffirently and not abuse CallMapper struct
		cm.Stop = true
	}
}

func (cm *CallMapper) BFS(start *callgraph.Node, initialPath []string, paths *match.CallPaths, options Options) {
	queue := list.New()
	queue.PushBack(BFSNode{Node: start, Path: initialPath})

	pathLimited := false
	for queue.Len() > 0 {
		// we process the first node - on first iteration, it'd be [Normalize] cogs/cogs.go:156:19 --->
		bfsNodeElm := queue.Front()
		// We remove last elm so we can put it in the front after updating it with new paths
		queue.Remove(bfsNodeElm)

		current := bfsNodeElm.Value.(BFSNode)
		currentNode := current.Node
		currentPath := current.Path

		if (currentNode.Func.Name() == "main" || currentNode.Func.Name() == "main$1") && !options.ContinueAfterMain {
			paths.InsertPaths(currentPath, false, false)
			continue
		}

		// Are we out of nodes for this currentNode, or have we reached the limit of funcs in a path?
		limitFuncs := options.MaxFuncs > 0 && len(currentPath) >= options.MaxFuncs
		if len(currentNode.In) == 0 || limitFuncs {
			paths.InsertPaths(currentPath, limitFuncs, false)
			continue
		}

		newPath := appendNodeToPath(currentNode, currentPath, options, nil)

		allOutsideModule := true
		for _, e := range currentNode.In {
			// Do we care about this node, or is it in the path already (if it calls itself)?
			if callerInPath(e, newPath) {
				continue
			}
			if passesFilter(e.Caller, options.Filter) {
				// We care. So let's create a copy of the path. On first iteration this has only our two intial nodes
				newPathCopy := make([]string, len(newPath))
				copy(newPathCopy, newPath)

				// We want to process the new node we added to the path.
				newPathWithCaller := appendNodeToPath(e.Caller, newPathCopy, options, e.Site)
				queue.PushBack(BFSNode{Node: e.Caller, Path: newPathWithCaller})
				allOutsideModule = false
			} else {
				continue
			}

			if options.MaxPaths > 0 && queue.Len()+len(paths.Paths) >= options.MaxPaths {
				pathLimited = true
				break
			}
		}
		if allOutsideModule {
			paths.InsertPaths(currentPath, false, true)
		}
	}

	// Insert whataver is left by now
	for e := queue.Front(); e != nil; e = e.Next() {
		bfsNode := e.Value.(BFSNode)
		paths.InsertPaths(bfsNode.Path, false, false)
		cm.Match.SSA.PathLimited = pathLimited
	}
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
	if site == nil {
		return path
		//return append(path, fmt.Sprintf("[%s] %s", s.Func.Name(), wallylib.GetFormattedPos(s.Func.Package(), s.Func.Pos())))
	}

	if options.PrintNodes || s.Func.Package() == nil {
		return append(path, s.String())
	}

	fp := wallylib.GetFormattedPos(s.Func.Package(), site.Pos())
	nodeDescription := fmt.Sprintf("%s.[%s] %s", s.Func.Pkg.Pkg.Name(), s.Func.Name(), fp)

	if s.Func.Recover != nil {
		hasRecover, err := findDeferRecover(s.Func, s.Func.Recover.Index-1)
		if err != nil {
			nodeDescription = fmt.Sprintf("%s.[%s] (%s) %s", s.Func.Pkg.Pkg.Name(), s.Func.Name(), err.Error(), fp)
		} else if hasRecover {
			nodeDescription = fmt.Sprintf("%s.[%s] (recoverable) %s", s.Func.Pkg.Pkg.Name(), s.Func.Name(), fp)
		}
	}
	return append(path, nodeDescription)
}

func passesFilter(node *callgraph.Node, filter string) bool {
	if node.Func != nil && node.Func.Pkg != nil {
		return strings.HasPrefix(node.Func.Pkg.Pkg.Path(), filter) || node.Func.Pkg.Pkg.Path() == "main"
	}
	return false
}

func findDeferRecover(fn *ssa.Function, idx int) (bool, error) {
	visited := make(map[*ssa.Function]bool)
	return findDeferRecoverRecursive(fn, visited, idx)
}

func findDeferRecoverRecursive(fn *ssa.Function, visited map[*ssa.Function]bool, starterBlock int) (bool, error) {
	if visited[fn] {
		return false, nil
	}

	visited[fn] = true

	// we use starterBlock on first call as we know where the defer call is, then reset it to 0 for subsequent blocks
	// to find the recover() if there
	for blockIdx := starterBlock; blockIdx < len(fn.Blocks); blockIdx++ {
		block := fn.Blocks[blockIdx]
		for _, instr := range block.Instrs {
			switch it := instr.(type) {
			case *ssa.Defer:
				if call, ok := it.Call.Value.(*ssa.Function); ok {
					if containsRecoverCall(call) {
						return true, nil
					}
				}
			case *ssa.Go:
				if call, ok := it.Call.Value.(*ssa.Function); ok {
					if containsRecoverCall(call) {
						return true, nil
					}
				}
			case *ssa.Call:
				if callee := it.Call.Value; callee != nil {
					if callee.Name() == "recover" {
						return true, nil
					}
				}
			case *ssa.MakeClosure:
				if closureFn, ok := it.Fn.(*ssa.Function); ok {
					res, err := findDeferRecoverRecursive(closureFn, visited, 0)
					if err != nil {
						return false, errors.New("Unexpected error finding recover block")
					}
					if res {
						return true, nil
					}
				}
			}
		}
	}
	return false, nil
}

func containsRecoverCall(fn *ssa.Function) bool {
	for _, block := range fn.Blocks {
		for _, instr := range block.Instrs {
			if isRecoverCall(instr) {
				return true
			}
		}
	}
	return false
}

func isRecoverCall(instr ssa.Instruction) bool {
	if callInstr, ok := instr.(*ssa.Call); ok {
		if callee, ok := callInstr.Call.Value.(*ssa.Builtin); ok {
			return callee.Name() == "recover"
		}
	}
	return false
}

// Only to be used when debugging
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
