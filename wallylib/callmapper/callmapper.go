package callmapper

import (
	"container/list"
	"fmt"
	"github.com/hex0punk/wally/match"
	"github.com/hex0punk/wally/wallylib"
	"github.com/hex0punk/wally/wallynode"
	"go/token"
	"golang.org/x/tools/go/callgraph"
	"golang.org/x/tools/go/ssa"
	"strings"
)

type SearchAlgorithm int

const (
	Bfs SearchAlgorithm = iota
	Dfs
)

type CallMapper struct {
	Options        Options
	Match          *match.RouteMatch
	Stop           bool
	CallgraphNodes map[*ssa.Function]*callgraph.Node
}

var SearchAlgs = map[string]SearchAlgorithm{
	"bfs": Bfs,
	"dfs": Dfs,
}

// TODO: this should be path of the callpath structs in match pkg
type BFSNode struct {
	Node *callgraph.Node
	Path []wallynode.WallyNode
	//Path []string
}

type LimiterMode int

// None = allows analysis to run pass main
// Normal = filters up to the main function if possible unless. It also filters up to main pkg *unless* the last function before going outside of main is a closure
// Strict = Does not allow going past the main package
const (
	None LimiterMode = iota
	Normal
	High
	Strict
	VeryStrict
)

var LimiterModes = map[string]LimiterMode{
	"none":        None,
	"normal":      Normal,
	"high":        High,
	"strict":      Strict,
	"very-strict": VeryStrict,
}

type Options struct {
	Filter       string
	MaxFuncs     int
	MaxPaths     int
	PrintNodes   bool
	SearchAlg    SearchAlgorithm
	Limiter      LimiterMode
	SkipClosures bool
	ModuleOnly   bool
	Simplify     bool
}

func NewCallMapper(match *match.RouteMatch, nodes map[*ssa.Function]*callgraph.Node, options Options) *CallMapper {
	// Rather than adding another state to check for, this is easier
	if options.ModuleOnly && match.Module != "" {
		options.Filter = match.Module
	}
	return &CallMapper{
		Options:        options,
		Match:          match,
		CallgraphNodes: nodes,
	}
}

func (cm *CallMapper) initPath(s *callgraph.Node) []wallynode.WallyNode {
	encPkg := cm.Match.SSA.EnclosedByFunc.Pkg
	encBasePos := wallylib.GetFormattedPos(encPkg, cm.Match.SSA.EnclosedByFunc.Pos())
	rec := wallynode.IsRecoverable(s, cm.CallgraphNodes)
	encStr := cm.getNodeString(encBasePos, s, rec)

	if cm.Options.Simplify {
		cm.Match.SSA.TargetPos = encStr
		return []wallynode.WallyNode{}
	}

	// TODO: No real reason for this to be here
	siteStr := ""
	if cm.Match.SSA.SSAInstruction == nil {
		encStr = cm.Match.Pos.String()
	} else {
		sitePkg := cm.Match.SSA.SSAInstruction.Parent().Pkg

		// cm.Options.Simplify should be false if here
		siteBasePos := wallylib.GetFormattedPos(sitePkg, cm.Match.SSA.SSAInstruction.Pos())
		if cm.Match.SSA.SSAFunc == nil {
			siteStr = fmt.Sprintf("%s.[%s] %s", sitePkg.Pkg.Name(), cm.Match.Indicator.Function, siteBasePos)
		} else {
			targetFuncNode := cm.CallgraphNodes[cm.Match.SSA.SSAFunc]
			isRec := wallynode.IsRecoverable(targetFuncNode, cm.CallgraphNodes)
			siteStr = cm.getNodeString(siteBasePos, targetFuncNode, isRec)
		}
		cm.Match.SSA.TargetPos = siteStr
	}

	initialPath := []wallynode.WallyNode{
		{NodeString: encStr, Caller: s},
	}
	return initialPath
}

func (cm *CallMapper) AllPathsBFS(s *callgraph.Node) *match.CallPaths {
	initialPath := cm.initPath(s)
	callPaths := &match.CallPaths{}
	cm.BFS(s, initialPath, callPaths)
	return callPaths
}

func (cm *CallMapper) AllPathsDFS(s *callgraph.Node) *match.CallPaths {
	visited := make(map[int]bool)
	initialPath := cm.initPath(s)
	callPaths := &match.CallPaths{}
	callPaths.Paths = []*match.CallPath{}
	cm.DFS(s, visited, initialPath, callPaths, nil)
	return callPaths
}

func (cm *CallMapper) DFS(destination *callgraph.Node, visited map[int]bool, path []wallynode.WallyNode, paths *match.CallPaths, site ssa.CallInstruction) {
	if cm.Options.Limiter > None && destination.Func.Pos() == token.NoPos {
		return
	}
	newPath := cm.appendNodeToPath(destination, path, site)

	if cm.Options.Limiter > None && isMainFunc(destination) {
		paths.InsertPaths(newPath, false, false, cm.Options.Simplify)
		cm.Stop = false
		return
	}

	mustStop := cm.Options.MaxFuncs > 0 && len(newPath) >= cm.Options.MaxFuncs
	if len(destination.In) == 0 || mustStop || cm.Stop {
		paths.InsertPaths(newPath, mustStop, cm.Stop, cm.Options.Simplify)
		cm.Stop = false
		return
	}

	// Avoids recursion within a single callpath
	if visited[destination.ID] {
		paths.InsertPaths(newPath, false, false, cm.Options.Simplify)
		return
	}
	visited[destination.ID] = true

	defer delete(visited, destination.ID)

	cm.Stop = false
	allOutsideModule := true
	allOutsideMainPkg := true

	fnT := destination
	if cm.Options.Limiter >= Strict || cm.Options.SkipClosures {
		fnT, newPath = cm.handleClosure(destination, newPath)
	}
	for _, e := range fnT.In {
		if e.Caller.Func.Package() == nil {
			continue
		}
		if paths.Paths != nil && cm.Options.MaxPaths > 0 && len(paths.Paths) >= cm.Options.MaxPaths {
			cm.Match.SSA.PathLimited = true
			continue
		}
		if visited[e.Caller.ID] {
			continue
		}

		if !shouldSkipNode(e, fnT, cm.Options) {
			if mainPkgLimited(fnT, e, cm.Options) {
				continue
			}
			allOutsideMainPkg = false
			allOutsideModule = false
			cm.DFS(e.Caller, visited, newPath, paths, e.Site)
		}
	}
	if allOutsideModule {
		// TODO: This is a quick and dirty solution to marking a path as going outside the module
		// This should be handled diffirently and not abuse CallMapper struct
		cm.Stop = true
	}
	if allOutsideMainPkg {
		paths.InsertPaths(newPath, mustStop, cm.Stop, cm.Options.Simplify)
		cm.Stop = false
		return
	}
}

func (cm *CallMapper) BFS(start *callgraph.Node, initialPath []wallynode.WallyNode, paths *match.CallPaths) {
	queue := list.New()
	queue.PushBack(BFSNode{Node: start, Path: initialPath})

	pathLimited := false
	for queue.Len() > 0 {
		//printQueue(queue)
		// we process the first node
		bfsNodeElm := queue.Front()
		// We remove last elm, so we can put it in the front after updating it with new paths
		queue.Remove(bfsNodeElm)

		current := bfsNodeElm.Value.(BFSNode)
		currentNode := current.Node
		currentPath := current.Path
		//printQueue(queue)

		if cm.Options.Limiter > None && currentNode.Func.Pos() == token.NoPos {
			paths.InsertPaths(currentPath, false, false, cm.Options.Simplify)
			continue
		}

		if cm.Options.Limiter > None && isMainFunc(currentNode) {
			paths.InsertPaths(currentPath, false, false, cm.Options.Simplify)
			continue
		}

		// Are we out of nodes for this currentNode, or have we reached the limit of funcs in a path?
		if limitFuncsReached(currentPath, cm.Options) {
			paths.InsertPaths(currentPath, true, false, cm.Options.Simplify)
			continue
		}

		var newPath []wallynode.WallyNode
		iterNode := currentNode
		if cm.Options.Limiter >= Strict || cm.Options.SkipClosures {
			iterNode, newPath = cm.handleClosure(currentNode, currentPath)
		} else {
			newPath = cm.appendNodeToPath(currentNode, currentPath, nil)
		}

		allOutsideFilter, allOutsideMainPkg, allAlreadyInPath := true, true, true
		allMismatchSite := true
		for _, e := range iterNode.In {
			if e.Caller.Func.Package() == nil {
				continue
			}
			if e.Site == nil {
				continue
			}
			// Do we care about this node, or is it in the path already (if it calls itself)?
			if cm.callerInPath(e, newPath) {
				continue
			}
			if cm.Options.Limiter >= VeryStrict {
				// make sure that site matches the function of the current node
				if !wallylib.SiteMatchesFunc(e.Site, iterNode.Func) {
					allMismatchSite = false
					allAlreadyInPath = false
					continue
				}
			}
			if cm.Options.Filter == "" || passesFilter(e.Caller, cm.Options.Filter) {
				if mainPkgLimited(iterNode, e, cm.Options) {
					allAlreadyInPath = false
					continue
				}

				allMismatchSite = false
				allOutsideMainPkg = false
				allOutsideFilter = false
				allAlreadyInPath = false
				// We care. So let's create a copy of the path. On first iteration this has only our two intial nodes
				newPathCopy := make([]wallynode.WallyNode, len(newPath))
				copy(newPathCopy, newPath)

				// We want to process the new node we added to the path.
				newPathWithCaller := cm.appendNodeToPath(e.Caller, newPathCopy, e.Site)
				queue.PushBack(BFSNode{Node: e.Caller, Path: newPathWithCaller})
				//printQueue(queue)
				// Have we reached the max paths set by the user
				if cm.Options.MaxPaths > 0 && queue.Len()+len(paths.Paths) >= cm.Options.MaxPaths {
					pathLimited = true
					break
				}
			}
		}
		if allOutsideMainPkg && !allAlreadyInPath {
			paths.InsertPaths(newPath, false, false, cm.Options.Simplify)
			continue
		}
		if cm.Options.Filter != "" && allOutsideFilter {
			paths.InsertPaths(newPath, false, true, cm.Options.Simplify)
			continue
		}
		if allMismatchSite {
			paths.InsertPaths(currentPath, false, false, cm.Options.Simplify)
			continue
		}
		if allAlreadyInPath {
			paths.InsertPaths(newPath, false, false, cm.Options.Simplify)
		}
	}

	// Insert whataver is left by now
	for e := queue.Front(); e != nil; e = e.Next() {
		bfsNode := e.Value.(BFSNode)
		paths.InsertPaths(bfsNode.Path, false, false, cm.Options.Simplify)
		cm.Match.SSA.PathLimited = pathLimited
	}
}

func limitFuncsReached(path []wallynode.WallyNode, options Options) bool {
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

	// This occurs if we are at init
	if currentNode.Func.Pos() == token.NoPos {
		return true
	}

	currentPkg := currentNode.Func.Package().Pkg
	callerPkg := e.Caller.Func.Package().Pkg

	if currentPkg.Name() != "main" {
		return false
	}

	isDifferentMainPkg := callerPkg.Name() == "main" && currentPkg.Path() != callerPkg.Path()
	isNonMainPkg := callerPkg.Name() != "main" && currentPkg.Path() != callerPkg.Path()
	isNonMainCallerOrClosure := isNonMainPkg && !wallylib.IsClosure(currentNode.Func)

	if options.Limiter == Normal {
		return isDifferentMainPkg || isNonMainCallerOrClosure
	}

	if options.Limiter >= High {
		return isDifferentMainPkg || isNonMainPkg
	}
	return false
}

func shouldSkipNode(e *callgraph.Edge, destination *callgraph.Node, options Options) bool {
	if options.Limiter >= VeryStrict {
		// make sure that site matches the function of the current node
		if !wallylib.SiteMatchesFunc(e.Site, destination.Func) {
			return true
		}
	}
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

func (cm *CallMapper) callerInPath(e *callgraph.Edge, paths []wallynode.WallyNode) bool {
	for _, p := range paths {
		if e.Caller.ID == p.Caller.ID {
			return true
		}
	}
	return false
}

func (cm *CallMapper) appendNodeToPath(s *callgraph.Node, path []wallynode.WallyNode, site ssa.CallInstruction) []wallynode.WallyNode {
	if site == nil {
		if cm.Options.Simplify {
			s = cm.getClosureRootNode(s)
			return append(path, wallynode.NewWallyNode("", s, site, cm.CallgraphNodes))
		}
		return path
	}

	if cm.Options.PrintNodes || s.Func.Package() == nil {
		return append(path, wallynode.NewWallyNode(s.String(), s, site, cm.CallgraphNodes))
	}

	return append(path, wallynode.NewWallyNode("", s, site, cm.CallgraphNodes))
}

func (cm *CallMapper) getNodeString(basePos string, s *callgraph.Node, recoverable bool) string {
	pkg := s.Func.Package()
	function := s.Func
	baseStr := fmt.Sprintf("%s.[%s] %s", pkg.Pkg.Name(), function.Name(), basePos)

	if recoverable {
		return fmt.Sprintf("%s.[%s] (recoverable) %s", pkg.Pkg.Name(), function.Name(), basePos)
	}

	return baseStr
}

func (cm *CallMapper) handleClosure(node *callgraph.Node, currentPath []wallynode.WallyNode) (*callgraph.Node, []wallynode.WallyNode) {
	newPath := cm.appendNodeToPath(node, currentPath, nil)

	if wallylib.IsClosure(node.Func) {
		node = cm.CallgraphNodes[node.Func.Parent()]
		for wallylib.IsClosure(node.Func) {
			if !cm.Options.Simplify {
				str := fmt.Sprintf("%s.[%s] %s", node.Func.Pkg.Pkg.Name(), node.Func.Name(), wallylib.GetFormattedPos(node.Func.Package(), node.Func.Pos()))
				newPath = append(newPath, wallynode.NewWallyNode(str, node, nil, cm.CallgraphNodes))
				//str := fmt.Sprintf("%s.[%s] %s", node.Func.Pkg.Pkg.Name(), node.Func.Name(), wallylib.GetFormattedPos(node.Func.Package(), node.Func.Pos()))
				//newPath = append(newPath, wallynode.WallyNode{
				//	NodeString: str,
				//	Caller:     node,
				//})
			}
			node = cm.CallgraphNodes[node.Func.Parent()]
		}
	}

	return node, newPath
}

func (cm *CallMapper) buildWallyNode(s *callgraph.Node, site ssa.CallInstruction) wallynode.WallyNode {
	if site == nil {
		if cm.Options.Simplify {
			s = cm.getClosureRootNode(s)
		}
		return wallynode.NewWallyNode("", s, site, cm.CallgraphNodes)
	}

	if cm.Options.PrintNodes || s.Func.Package() == nil {
		return wallynode.NewWallyNode(s.String(), s, site, cm.CallgraphNodes)
	}

	return wallynode.NewWallyNode("", s, site, cm.CallgraphNodes)
}

func (cm *CallMapper) getClosureRootNode(s *callgraph.Node) *callgraph.Node {
	if wallylib.IsClosure(s.Func) {
		node := cm.CallgraphNodes[s.Func.Parent()]
		for wallylib.IsClosure(node.Func) {
			node = cm.CallgraphNodes[node.Func.Parent()]
		}
		return node
	}
	return s
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
