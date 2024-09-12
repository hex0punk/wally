package wallynode

import (
	"errors"
	"fmt"
	"golang.org/x/tools/go/callgraph"
	"golang.org/x/tools/go/ssa"
)

type WallyNode struct {
	NodeString  string
	Caller      *callgraph.Node
	Site        ssa.CallInstruction
	recoverable bool
}

type NodeType int

const (
	Site NodeType = iota
	Function
)

//func NewWallyNode(nodeStr string, caller *callgraph.Node, site ssa.CallInstruction, connectedNodes map[*ssa.Function]*callgraph.Node) WallyNode {
//	recoverable := false
//	if nodeStr == "" {
//		if site == nil {
//			nodeStr = fmt.Sprintf("Func: %s.[%s] %s", caller.Func.Pkg.Pkg.Name(), caller.Func.Name(), wallylib.GetFormattedPos(caller.Func.Package(), caller.Func.Pos()))
//		} else {
//			fp := wallylib.GetFormattedPos(caller.Func.Package(), site.Pos())
//			recoverable = IsRecoverable(caller, connectedNodes)
//			nodeStr = GetNodeString(fp, caller, recoverable)
//		}
//	}
//	return WallyNode{
//		NodeString:  nodeStr,
//		Caller:      caller,
//		Site:        site,
//		recoverable: recoverable,
//	}
//}

func (n *WallyNode) IsRecoverable() bool {
	return n.recoverable
}

func GetNodeString(basePos string, s *callgraph.Node, recoverable bool) string {
	pkg := s.Func.Package()
	function := s.Func
	baseStr := fmt.Sprintf("%s.[%s] %s", pkg.Pkg.Name(), function.Name(), basePos)

	if recoverable {
		return fmt.Sprintf("%s.[%s] (recoverable) %s", pkg.Pkg.Name(), function.Name(), basePos)
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
					if nestedFunc, ok := callee.(*ssa.Function); ok {
						if _, err := findDeferRecoverRecursive(nestedFunc, visited, 0); err != nil {
							return true, nil
						}
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

// closureArgumentOf checks if the function is passed as an argument to another function
// and returns the enclosing function
func closureArgumentOf(targetNode *callgraph.Node, edges *callgraph.Node) *ssa.Function {
	for _, edge := range edges.Out {
		for _, arg := range edge.Site.Common().Args {
			if argFn, ok := arg.(*ssa.MakeClosure); ok {
				if argFn.Fn == targetNode.Func {
					if res, ok := edge.Site.Common().Value.(*ssa.Function); ok {
						return res
					}
				}
			}
		}
	}
	return nil
}
