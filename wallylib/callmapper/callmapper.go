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

type LimiterMode int

// None = allows analysis to run pass main
// Normal = filters up to the main function if possible unless. It also filters up to main pkg *unless* the last function before going outside of main is a closure
// Strict = Does not allow going past the main package
const (
	None LimiterMode = iota
	Normal
	Strict
)

var LimiterModes = map[string]LimiterMode{
	"none":   None,
	"normal": Normal,
	"strict": Strict,
}

type Options struct {
	Filter     string
	MaxFuncs   int
	MaxPaths   int
	PrintNodes bool
	SearchAlg  SearchAlgorithm
	Limiter    LimiterMode
}

func NewCallMapper(match *match.RouteMatch, options Options) *CallMapper {
	return &CallMapper{
		Options: options,
		Match:   match,
	}
}

func (cm *CallMapper) initPath() []string {
	encPkg := cm.Match.SSA.EnclosedByFunc.Pkg
	encBasePos := wallylib.GetFormattedPos(encPkg, cm.Match.SSA.EnclosedByFunc.Pos())
	encStr := getNodeString(encBasePos, encPkg, cm.Match.SSA.EnclosedByFunc)

	// TODO: No real reason for this to be here
	siteStr := ""
	if cm.Match.SSA.SSAInstruction == nil {
		encStr = cm.Match.Pos.String()
	} else {
		sitePkg := cm.Match.SSA.SSAInstruction.Parent().Pkg
		siteBasePos := wallylib.GetFormattedPos(sitePkg, cm.Match.SSA.SSAInstruction.Pos())
		if cm.Match.SSA.SSAFunc == nil {
			siteStr = fmt.Sprintf("%s.[%s] %s", sitePkg.Pkg.Name(), cm.Match.Indicator.Function, siteBasePos)
		} else {
			siteStr = getNodeString(siteBasePos, sitePkg, cm.Match.SSA.SSAFunc)
		}
		cm.Match.SSA.TargetPos = siteStr
	}

	initialPath := []string{
		encStr,
	}
	return initialPath
}

func (cm *CallMapper) AllPathsBFS(s *callgraph.Node, options Options) *match.CallPaths {
	initialPath := cm.initPath()
	callPaths := &match.CallPaths{}
	cm.BFS(s, initialPath, callPaths, options)
	return callPaths
}

func (cm *CallMapper) AllPathsDFS(s *callgraph.Node, options Options) *match.CallPaths {
	visited := make(map[int]bool)
	initialPath := cm.initPath()
	callPaths := &match.CallPaths{}
	callPaths.Paths = []*match.CallPath{}
	cm.DFS(s, visited, initialPath, callPaths, options, nil)
	return callPaths
}

func (cm *CallMapper) DFS(destination *callgraph.Node, visited map[int]bool, path []string, paths *match.CallPaths, options Options, site ssa.CallInstruction) {
	newPath := appendNodeToPath(destination, path, options, site)
	if options.Limiter > 0 && isMainFunc(destination) {
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

	cm.Stop = false
	allOutsideModule := true
	allOutsideMainPkg := true
	for _, e := range destination.In {
		if paths.Paths != nil && options.MaxPaths > 0 && len(paths.Paths) >= options.MaxPaths {
			cm.Match.SSA.PathLimited = true
			continue
		}
		if visited[e.Caller.ID] {
			continue
		}

		if !shouldSkipNode(e, options) {
			if mainPkgLimited(destination, e, options) {
				continue
			}
			allOutsideMainPkg = false
			allOutsideModule = false
			cm.DFS(e.Caller, visited, newPath, paths, options, e.Site)
		}
	}
	if allOutsideModule {
		// TODO: This is a quick and dirty solution to marking a path as going outside the module
		// This should be handled diffirently and not abuse CallMapper struct
		cm.Stop = true
	}
	if allOutsideMainPkg {
		paths.InsertPaths(newPath, mustStop, cm.Stop)
		cm.Stop = false
		return
	}
}

func (cm *CallMapper) BFS(start *callgraph.Node, initialPath []string, paths *match.CallPaths, options Options) {
	queue := list.New()
	queue.PushBack(BFSNode{Node: start, Path: initialPath})

	pathLimited := false
	for queue.Len() > 0 {
		//printQueue(queue)
		// we process the first node
		bfsNodeElm := queue.Front()
		// We remove last elm so we can put it in the front after updating it with new paths
		queue.Remove(bfsNodeElm)

		current := bfsNodeElm.Value.(BFSNode)
		currentNode := current.Node
		currentPath := current.Path

		if options.Limiter > None && isMainFunc(currentNode) {
			paths.InsertPaths(currentPath, false, false)
			continue
		}

		// Are we out of nodes for this currentNode, or have we reached the limit of funcs in a path?
		if limitFuncsReached(currentPath, options) {
			paths.InsertPaths(currentPath, true, false)
			continue
		}

		newPath := appendNodeToPath(currentNode, currentPath, options, nil)

		allOutsideFilter, allOutsideMainPkg, allAlreadyInPath := true, true, true
		for _, e := range currentNode.In {
			// Do we care about this node, or is it in the path already (if it calls itself)?
			if callerInPath(e, newPath) {
				continue
			}
			if options.Filter == "" || passesFilter(e.Caller, options.Filter) {
				if mainPkgLimited(currentNode, e, options) {
					allAlreadyInPath = false
					continue
				}
				allOutsideMainPkg = false
				allOutsideFilter = false
				allAlreadyInPath = false
				// We care. So let's create a copy of the path. On first iteration this has only our two intial nodes
				newPathCopy := make([]string, len(newPath))
				copy(newPathCopy, newPath)

				// We want to process the new node we added to the path.
				newPathWithCaller := appendNodeToPath(e.Caller, newPathCopy, options, e.Site)
				queue.PushBack(BFSNode{Node: e.Caller, Path: newPathWithCaller})

				// Have we reached the max paths set by the user
				if options.MaxPaths > 0 && queue.Len()+len(paths.Paths) >= options.MaxPaths {
					pathLimited = true
					break
				}
			}
		}
		if allOutsideMainPkg && !allAlreadyInPath {
			paths.InsertPaths(currentPath, false, false)
			continue
		}
		if options.Filter != "" && allOutsideFilter {
			paths.InsertPaths(currentPath, false, true)
			continue
		}
		if allAlreadyInPath {
			paths.InsertPaths(currentPath, false, false)
		}
	}

	// Insert whataver is left by now
	for e := queue.Front(); e != nil; e = e.Next() {
		bfsNode := e.Value.(BFSNode)
		paths.InsertPaths(bfsNode.Path, false, false)
		cm.Match.SSA.PathLimited = pathLimited
	}
}

func limitFuncsReached(path []string, options Options) bool {
	return options.MaxFuncs > 0 && len(path) >= options.MaxFuncs
}

func isMainFunc(node *callgraph.Node) bool {
	return node.Func.Name() == "main" || strings.HasPrefix(node.Func.Name(), "main$")
}

// Used to help wrangle some of the unrealistic resutls from cha.Callgraph
func mainPkgLimited(currentNode *callgraph.Node, e *callgraph.Edge, options Options) bool {
	if options.Limiter == None {
		return false
	}

	currentPkg := currentNode.Func.Package().Pkg
	callerPkg := e.Caller.Func.Package().Pkg

	if currentPkg.Name() != "main" {
		return false
	}

	isDifferentMainPkg := callerPkg.Name() == "main" && currentPkg.Path() != callerPkg.Path()
	isNonMainPkg := callerPkg.Name() != "main"
	isNonMainCallerOrClosure := isNonMainPkg && !strings.Contains(currentNode.Func.Name(), "$")

	if options.Limiter == Normal {
		return isDifferentMainPkg || isNonMainCallerOrClosure
	}

	if options.Limiter == Strict {
		return isDifferentMainPkg || isNonMainPkg
	}
	return false
}

func shouldSkipNode(e *callgraph.Edge, options Options) bool {
	if options.Filter != "" && e.Caller != nil && !passesFilter(e.Caller, options.Filter) {
		return true
	}
	return false
}

func passesFilter(node *callgraph.Node, filter string) bool {
	if node.Func != nil && node.Func.Pkg != nil {
		return strings.HasPrefix(node.Func.Pkg.Pkg.Path(), filter) || node.Func.Pkg.Pkg.Path() == "main"
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
		//return append(path, fmt.Sprintf("Func: %s.[%s] %s", s.Func.Pkg.Pkg.Name(), s.Func.Name(), wallylib.GetFormattedPos(s.Func.Package(), s.Func.Pos())))
	}

	if options.PrintNodes || s.Func.Package() == nil {
		return append(path, s.String())
	}

	fp := wallylib.GetFormattedPos(s.Func.Package(), site.Pos())

	nodeDescription := getNodeString(fp, s.Func.Pkg, s.Func)

	return append(path, nodeDescription)
}

func getNodeString(basePos string, pkg *ssa.Package, function *ssa.Function) string {
	baseStr := ""
	baseStr = fmt.Sprintf("%s.[%s] %s", pkg.Pkg.Name(), function.Name(), basePos)
	if function.Recover != nil {
		rec, err := findDeferRecover(function, function.Recover.Index-1)
		if err != nil {
			baseStr = fmt.Sprintf("%s.[%s] (%s) %s", pkg.Pkg.Name(), function.Name(), err.Error(), basePos)
		}
		if rec {
			baseStr = fmt.Sprintf("%s.[%s] (recoverable) %s", pkg.Pkg.Name(), function.Name(), basePos)
		}
	}
	return baseStr
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
						return false, errors.New("unexpected error finding recover block")
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
