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
			recoverable = IsRecoverable(caller, f.CallgraphNodes)
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
