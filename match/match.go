package match

import (
	"go/token"
	"go/types"
	"golang.org/x/tools/go/callgraph"
	"golang.org/x/tools/go/ssa"
	"wally/indicator"
)

type RouteMatch struct {
	Indicator  indicator.Indicator // It should be FuncInfo instead
	Params     map[string]string
	Pos        token.Position
	Signature  *types.Signature
	EnclosedBy string
	SSA        *SSAContext
}

// TODO: I don't love this here, maybe an SSA dedicated pkg would be better
type SSAContext struct {
	RecLimited     bool
	EnclosedByFunc *ssa.Function
	Edges          []*callgraph.Edge
	CallPaths      [][]string
}

func NewRouteMatch(indicator indicator.Indicator, pos token.Position) RouteMatch {
	return RouteMatch{
		Indicator: indicator,
		Pos:       pos,
		SSA:       &SSAContext{},
	}
}
