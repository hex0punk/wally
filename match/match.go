package match

import (
	"encoding/json"
	"github.com/google/uuid"
	"go/token"
	"go/types"
	"golang.org/x/tools/go/callgraph"
	"golang.org/x/tools/go/ssa"
	"wally/indicator"
)

type RouteMatch struct {
	Id         string
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
		Id:        uuid.New().String(),
		Indicator: indicator,
		Pos:       pos,
		SSA:       &SSAContext{},
	}
}
func (r *RouteMatch) MarshalJSON() ([]byte, error) {
	var enclosedBy string
	if r.SSA != nil && r.SSA.EnclosedByFunc != nil {
		enclosedBy = r.SSA.EnclosedByFunc.String()
	} else {
		enclosedBy = r.EnclosedBy
	}

	params := make(map[string]string)
	for k, v := range r.Params {
		if v == "" {
			v = "<could not resolve>"
		}
		if k == "" {
			k = "<not specified>"
		}
		params[k] = v
	}

	var resPaths [][]string
	for _, paths := range r.SSA.CallPaths {
		var p []string
		for x := len(paths) - 1; x >= 0; x-- {
			p = append(p, paths[x])
		}
		resPaths = append(resPaths, p)
	}

	return json.Marshal(struct {
		Indicator  indicator.Indicator
		Params     map[string]string
		Pos        string
		EnclosedBy string
		RecLimited bool
		Paths      [][]string
	}{
		Indicator:  r.Indicator,
		Params:     params,
		Pos:        r.Pos.String(),
		EnclosedBy: enclosedBy,
		RecLimited: r.SSA.RecLimited,
		Paths:      resPaths,
	})
}
