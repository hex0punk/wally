package match

import (
	"encoding/json"
	"fmt"
	"github.com/google/uuid"
	"github.com/hex0punk/wally/indicator"
	"github.com/hex0punk/wally/wallynode"
	"go/token"
	"go/types"
	"golang.org/x/tools/go/ssa"
)

type RouteMatch struct {
	MatchId    string
	Indicator  indicator.Indicator // It should be FuncInfo instead
	Params     map[string]string
	Pos        token.Position
	Signature  *types.Signature
	EnclosedBy string
	Module     string
	SSA        *SSAContext
}

// TODO: I don't love this here, maybe an SSA dedicated pkg would be better
type SSAContext struct {
	PathLimited    bool
	EnclosedByFunc *ssa.Function
	CallPaths      *CallPaths
	SSAInstruction ssa.CallInstruction
	SSAFunc        *ssa.Function
	TargetPos      string
}

type CallPaths struct {
	Paths []*CallPath
}

type CallPath struct {
	ID            int
	Nodes         []wallynode.WallyNode
	NodeLimited   bool
	FilterLimited bool
	Recoverable   bool
}

func (cp *CallPaths) InsertPaths(nodes []wallynode.WallyNode, nodeLimited bool, filterLimited bool, simplify bool) {
	callPath := CallPath{NodeLimited: nodeLimited, FilterLimited: filterLimited}

	for _, node := range nodes {
		if simplify && node.Site != nil {
			continue
		}
		callPath.Nodes = append(callPath.Nodes, node)
		// Temp hack while we replace nodes with a structure containing parts of a path (func, pkg, etc.)
		if node.IsRecoverable() {
			callPath.Recoverable = true
		}
	}

	// Simplified output can result in duplicates,
	// as there can be multiple call sites inside the same enclosing function
	if simplify {
		for _, existingPath := range cp.Paths {
			if isSamePath(existingPath, callPath.Nodes) {
				return
			}
		}
	}

	cp.Paths = append(cp.Paths, &callPath)
}

func isSamePath(callPath *CallPath, nodes []wallynode.WallyNode) bool {
	if len(callPath.Nodes) != len(nodes) {
		return false
	}

	for i, existingPath := range callPath.Nodes {
		if existingPath.NodeString != nodes[i].NodeString {
			return false
		}
	}

	return true
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
		p = append(p, r.SSA.TargetPos)
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
