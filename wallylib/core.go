package wallylib

import (
	"errors"
	"go/ast"
	"go/build"
	"go/types"
	"golang.org/x/tools/go/callgraph"
	"golang.org/x/tools/go/ssa"
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

type SSAContext struct {
	EnclosedByFunc *ssa.Function
	Edges          []*callgraph.Edge
	CallPaths      [][]string
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

// TODO: Lots of repeated code that we can refactor here
// Further, this is likely not sufficient if used for more general purposes (outside wally) as
// there are parts of some statements (i.e. a ForStmt Post) that are not handled here
func GetExprsFromStmt(stmt ast.Stmt) []*ast.CallExpr {
	var result []*ast.CallExpr
	switch s := stmt.(type) {
	case *ast.ExprStmt:
		ce := callExprFromExpr(s.X)
		if ce != nil {
			result = append(result, ce...)
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
			result = append(result, condCe...)
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
				result = append(result, ce...)
			}
		}
		for _, lhs := range s.Lhs {
			ce := callExprFromExpr(lhs)
			if ce != nil {
				result = append(result, ce...)
			}
		}
	case *ast.ReturnStmt:
		for _, retResult := range s.Results {
			ce := callExprFromExpr(retResult)
			if ce != nil {
				result = append(result, ce...)
			}
		}
	case *ast.ForStmt:
		ces := GetExprsFromStmt(s.Body)
		if ces != nil && len(ces) > 0 {
			result = append(result, ces...)
		}
	case *ast.RangeStmt:
		ces := GetExprsFromStmt(s.Body)
		if ces != nil && len(ces) > 0 {
			result = append(result, ces...)
		}
	case *ast.SelectStmt:
		for _, clause := range s.Body.List {
			//ces := GetExprsFromStmt(clause)
			if cc, ok := clause.(*ast.CommClause); ok {
				for _, stm := range cc.Body {
					bodyExps := GetExprsFromStmt(stm)
					if bodyExps != nil && len(bodyExps) > 0 {
						result = append(result, bodyExps...)
					}
				}
			}
		}
	case *ast.LabeledStmt:
		ces := GetExprsFromStmt(s.Stmt)
		if ces != nil && len(ces) > 0 {
			result = append(result, ces...)
		}
	}
	return result
}

func callExprFromExpr(e ast.Expr) []*ast.CallExpr {
	switch e := e.(type) {
	case *ast.CallExpr:
		return append([]*ast.CallExpr{}, e)
	case *ast.FuncLit:
		return GetExprsFromStmt(e.Body)
	}
	return nil
}

func inStd(node *callgraph.Node) bool {
	pkg, _ := build.Import(node.Func.Pkg.Pkg.Path(), "", 0)
	return pkg.Goroot
}
