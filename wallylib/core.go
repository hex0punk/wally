package wallylib

import (
	"errors"
	"go/ast"
	"go/build"
	"go/token"
	"go/types"
	"golang.org/x/tools/go/callgraph"
	"golang.org/x/tools/go/ssa"
	"strings"
	"wally/indicator"
)

type FuncInfo struct {
	Package   string
	Pkg       *types.Package
	Type      string
	Name      string
	Route     string
	Signature *types.Signature
}

type RouteMatch struct {
	Indicator  indicator.Indicator // It should be FuncInfo instead
	Params     map[string]string
	Pos        token.Position
	Signature  *types.Signature
	EnclosedBy string
	SSA        *SSAContext
}

type SSAContext struct {
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

func (fi *FuncInfo) Match(indicators []indicator.Indicator) *indicator.Indicator {
	var match *indicator.Indicator
	for _, ind := range indicators {
		ind := ind
		// User may decide they do not care if the package matches.
		// It'd be worth adding a command to "take a guess" for potential routes
		if fi.Package != ind.Package && ind.Package != "*" {
			continue
		}
		if fi.Name != ind.Function {
			continue
		}
		//if fi.Type != "" && fi.Type != ind.Type {
		//	continue
		//}
		match = &ind
	}
	return match
}

func GetFuncInfo(expr ast.Expr, info *types.Info) (*FuncInfo, error) {
	sel, ok := expr.(*ast.SelectorExpr)
	if !ok {
		return nil, errors.New("unable to get func data")
	}

	funcName := GetName(sel.Sel)
	pkgPath, err := ResolvePackageFromIdent(sel.Sel, info)
	if err != nil && funcName != "" {
		// Try to get pkg name from the selector, as this is likely not a pkg.func
		// but a struct.fun
		pkgPath, err = ResolvePackageFromIdent(sel.X, info)
		if err != nil {
			return nil, err
		}
	}

	return &FuncInfo{
		Package: pkgPath.Path(),
		Pkg:     pkgPath,
		//Type: nil,
		Name: funcName,
	}, nil
}

func GetFuncSignature(expr ast.Expr, info *types.Info) (*types.Signature, error) {
	idt, ok := expr.(*ast.Ident)
	if !ok {
		return nil, errors.New("not an ident for expr")
	}

	o1 := info.ObjectOf(idt)
	switch va := o1.(type) {
	case *types.Func:
		return va.Type().(*types.Signature), nil
	case *types.Var:
		// It's a function from a struct field
		return va.Type().(*types.Signature), nil
	default:
		return nil, errors.New("Unable to get signature")
	}
}

func GetName(e ast.Expr) string {
	ident, ok := e.(*ast.Ident)
	if !ok {
		return ""
	} else {
		return ident.Name
	}
}

func GetExprsFromStmt(stmt ast.Stmt) []*ast.CallExpr {
	var result []*ast.CallExpr
	switch s := stmt.(type) {
	case *ast.ExprStmt:
		ce := callExprFromExpr(s.X)
		if ce != nil {
			result = append(result, ce)
		}
	case *ast.SwitchStmt:
		for _, iclause := range s.Body.List {
			clause := iclause.(*ast.CaseClause)
			for _, stm := range clause.Body {
				bodyExps := GetExprsFromStmt(stm)
				if bodyExps != nil && len(bodyExps) > 0 {
					result = append(result, bodyExps...)
				}
			}
		}
	case *ast.IfStmt:
		condCe := callExprFromExpr(s.Cond)
		if condCe != nil {
			result = append(result, condCe)
		}
		if s.Init != nil {
			initCe := GetExprsFromStmt(s.Init)
			if initCe != nil && len(initCe) > 0 {
				result = append(result, initCe...)
			}
		}
		if s.Else != nil {
			elseCe := GetExprsFromStmt(s.Else)
			if elseCe != nil && len(elseCe) > 0 {
				result = append(result, elseCe...)
			}
		}
		ces := GetExprsFromStmt(s.Body)
		if ces != nil && len(ces) > 0 {
			result = append(result, ces...)
		}
	case *ast.BlockStmt:
		for _, stm := range s.List {
			ce := GetExprsFromStmt(stm)
			if ce != nil {
				result = append(result, ce...)
			}
		}
	case *ast.AssignStmt:
		for _, rhs := range s.Rhs {
			ce := callExprFromExpr(rhs)
			if ce != nil {
				result = append(result, ce)
			}
		}
		for _, lhs := range s.Lhs {
			ce := callExprFromExpr(lhs)
			if ce != nil {
				result = append(result, ce)
			}
		}

	}
	return result
}

func callExprFromExpr(e ast.Expr) *ast.CallExpr {
	switch e := e.(type) {
	case *ast.CallExpr:
		return e
	}
	return nil
}

func (r *RouteMatch) AllPaths(s *callgraph.Node, filter string, recLimit int) [][]string {
	visited := make(map[*callgraph.Node]bool)
	paths := [][]string{}
	path := []string{}

	r.DFS(s, visited, path, &paths, filter, recLimit)

	// TODO: We have to do this given that the cha callgraph algorithm seems to return duplicate paths at times.
	// I need to test other algorithms available to see if I get better results (without duplicate paths)
	res := dedupPaths(paths)

	return res
}

// TODO: this can be a generic function for deduping slices of slices
// and moved to a different package
func dedupPaths(paths [][]string) [][]string {
	var uniquePaths [][]string
	for x := range paths {
		match := true
		for y := range paths {
			if x == y {
				uniquePaths = append(uniquePaths, paths[x])
				continue
			}
			if (equal(paths[x], paths[y])) == false {
				match = false
			} else {
				match = true
				break
			}
		}
		if match == false {
			uniquePaths = append(paths, paths[x])
			//match = false
		}
	}
	return uniquePaths
}

// TODO: Move this to a more general use package
func equal(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i, x := range a {
		if x != b[i] {
			return false
		}
	}
	return true
}

func (r *RouteMatch) DFS(s *callgraph.Node, visited map[*callgraph.Node]bool, path []string, paths *[][]string, filter string, recLimit int) {
	visited[s] = true
	if !strings.HasSuffix(s.String(), "$bound") {
		if s.Func != nil {
			path = append(path, s.String())
		}
	}

	if len(s.In) == 0 {
		*paths = append(*paths, path)
	} else {
		for _, e := range s.In {
			if recLimit > 0 && len(*paths) >= recLimit {
				delete(visited, s)
				*paths = append(*paths, path)
				return
			}
			if filter != "" && e.Caller != nil {
				if !passesFilter(e.Caller, filter) {
					delete(visited, s)
					*paths = append(*paths, path)
					return
				}
			}
			if e.Caller != nil && !visited[e.Caller] {
				r.DFS(e.Caller, visited, path, paths, filter, recLimit)
			}
		}
	}

	delete(visited, s)
	//path = path[:len(path)-1]
}

func passesFilter(node *callgraph.Node, filter string) bool {
	if node.Func != nil && node.Func.Pkg != nil {
		return strings.HasPrefix(node.Func.Pkg.Pkg.Path(), filter)
	}
	return false
}

func inStd(node *callgraph.Node) bool {
	pkg, _ := build.Import(node.Func.Pkg.Pkg.Path(), "", 0)
	return pkg.Goroot
}
