package match

import (
	"encoding/json"
	"fmt"
	"github.com/google/uuid"
	"go/token"
	"go/types"
	"golang.org/x/tools/go/callgraph"
	"golang.org/x/tools/go/ssa"
	"strings"
	"wally/indicator"
)

type RouteMatch struct {
	MatchId    string
	Indicator  indicator.Indicator // It should be FuncInfo instead
	Params     map[string]string
	Pos        token.Position
	Signature  *types.Signature
	EnclosedBy string
	SSA        *SSAContext
}

// TODO: I don't love this here, maybe an SSA dedicated pkg would be better
type SSAContext struct {
	PathLimited    bool
	EnclosedByFunc *ssa.Function
	Edges          []*callgraph.Edge
	CallPaths      *CallPaths
}

type CallPaths struct {
	Paths []*CallPath
}

type CallPath struct {
	ID          int
	Nodes       []*Node
	NodeLimited bool
	Recoverable bool
}

type Node struct {
	NodeString string
	Pkg        *ssa.Package
	Func       *ssa.Function
}

func (cp *CallPaths) InsertPaths(nodes []string, nodeLimited bool) {
	callPath := CallPath{NodeLimited: nodeLimited}

	for _, node := range nodes {
		callPath.Nodes = append(callPath.Nodes, &Node{NodeString: node})
		// Temp hack while we replace nodes with a structure containing parts of a path (func, pkg, etc.)
		if strings.Contains(node, "(recoverable)") {
			callPath.Recoverable = true
		}
	}
	cp.Paths = append(cp.Paths, &callPath)
}

func (cp *CallPaths) Print() {
	for _, callPath := range cp.Paths {
		fmt.Println("NODE: ", callPath)
		for i, p := range callPath.Nodes {
			fmt.Printf("%d		Path: %s\n", i, p.NodeString)
		}
	}
}

func NewRouteMatch(indicator indicator.Indicator, pos token.Position) RouteMatch {
	return RouteMatch{
		MatchId:   uuid.New().String(),
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
	for _, paths := range r.SSA.CallPaths.Paths {
		var p []string
		for x := len(paths.Nodes) - 1; x >= 0; x-- {
			p = append(p, paths.Nodes[x].NodeString)
		}
		resPaths = append(resPaths, p)
	}

	return json.Marshal(struct {
		MatchId     string
		Indicator   indicator.Indicator
		Params      map[string]string
		Pos         string
		EnclosedBy  string
		PathLimited bool
		Paths       [][]string
	}{
		MatchId:     r.MatchId,
		Indicator:   r.Indicator,
		Params:      params,
		Pos:         r.Pos.String(),
		EnclosedBy:  enclosedBy,
		PathLimited: r.SSA.PathLimited,
		Paths:       resPaths,
	})
}
