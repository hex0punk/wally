package wallynode

import (
	"fmt"
	"github.com/hex0punk/wally/wallylib"
	"golang.org/x/tools/go/callgraph"
	"golang.org/x/tools/go/ssa"
)

type WallyNodeFactory struct {
	CallgraphNodes map[*ssa.Function]*callgraph.Node
}

func NewWallyNodeFactory(callGraphnodes map[*ssa.Function]*callgraph.Node) *WallyNodeFactory {
	return &WallyNodeFactory{
		CallgraphNodes: callGraphnodes,
	}
}

func (f *WallyNodeFactory) CreateWallyNode(nodeStr string, caller *callgraph.Node, site ssa.CallInstruction) WallyNode {
	recoverable := false
	if nodeStr == "" {
		if site == nil {
			nodeStr = fmt.Sprintf("Func: %s.[%s] %s", caller.Func.Pkg.Pkg.Name(), caller.Func.Name(), wallylib.GetFormattedPos(caller.Func.Package(), caller.Func.Pos()))
		} else {
			fp := wallylib.GetFormattedPos(caller.Func.Package(), site.Pos())
			recoverable = f.IsRecoverable(caller)
			nodeStr = GetNodeString(fp, caller, recoverable)
		}
	}
	return WallyNode{
		NodeString:  nodeStr,
		Caller:      caller,
		Site:        site,
		recoverable: recoverable,
	}
}

func (f *WallyNodeFactory) IsRecoverable(s *callgraph.Node) bool {
	function := s.Func
	if function.Recover != nil {
		rec, err := findDeferRecover(function, function.Recover.Index-1)
		if err == nil && rec {
			return true
		}
	}
	if wallylib.IsClosure(function) {
		enclosingFunc := closureArgumentOf(s, f.CallgraphNodes[s.Func.Parent()])
		if enclosingFunc != nil && enclosingFunc.Recover != nil {
			rec, err := findDeferRecover(enclosingFunc, enclosingFunc.Recover.Index-1)
			if err == nil && rec {
				return true
			}
		}
		if enclosingFunc != nil {
			for _, af := range enclosingFunc.AnonFuncs {
				if af.Recover != nil {
					rec, err := findDeferRecover(af, af.Recover.Index-1)
					if err == nil && rec {
						return true
					}
				}
			}
		}
	}
	return false
}
