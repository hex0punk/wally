package match

import (
	"crypto/sha1"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"go/token"
	"go/types"

	"github.com/google/uuid"
	"github.com/hex0punk/wally/wallylib"
	"github.com/hex0punk/wally/wallynode"
	"golang.org/x/tools/go/callgraph"
	"golang.org/x/tools/go/ssa"
)

type RouteMatch struct {
	MatchId     string
	IndicatorId string
	FuncInfo    *wallylib.FuncInfo
	Params      map[string]string
	Pos         token.Position
	Signature   *types.Signature
	EnclosedBy  string
	Module      string
	Hash        string
	SSA         *SSAContext
	ParentMatch *RouteMatch
}

// TODO: I don't love this here, maybe an SSA dedicated pkg would be better
type SSAContext struct {
	PathLimited    bool
	EnclosedByFunc *ssa.Function
	CallPaths      *CallPaths
	SSAInstruction ssa.CallInstruction
	SSAFunc        *ssa.Function
	TargetPos      string
	BaseNode       *callgraph.Node
	// Channels for progress reporting
	ProgressChan  chan int
	QueueSizeChan chan int
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

func NewRouteMatch(indicatorId string, funcInfo *wallylib.FuncInfo, pos token.Position) RouteMatch {
	objBytes, _ := json.Marshal(fmt.Sprintf("%s.%s", funcInfo.Pkg, funcInfo.Name))

	return RouteMatch{
		MatchId:     uuid.New().String(),
		IndicatorId: indicatorId,
		Pos:         pos,
		SSA:         &SSAContext{},
		Hash:        hash(objBytes),
		FuncInfo:    funcInfo,
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
		if r.ParentMatch != nil {
			p = append(p, r.ParentMatch.SSA.TargetPos)
		}
		resPaths = append(resPaths, p)
	}

	return json.Marshal(struct {
		MatchId     string
		IndicatorId string
		FuncName    string
		FuncPkg     string
		Params      map[string]string
		Pos         string
		EnclosedBy  string
		PathLimited bool
		Paths       [][]string
	}{
		MatchId:     r.MatchId,
		IndicatorId: r.IndicatorId,
		FuncName:    r.FuncInfo.Name,
		FuncPkg:     r.FuncInfo.Package,
		Params:      params,
		Pos:         r.Pos.String(),
		EnclosedBy:  enclosedBy,
		PathLimited: r.SSA.PathLimited,
		Paths:       resPaths,
	})
}

func hash(b []byte) string {
	if len(b) == 0 {
		return ""
	}

	h := sha1.New()
	h.Write(b)
	return base64.URLEncoding.EncodeToString(h.Sum(nil))
}
