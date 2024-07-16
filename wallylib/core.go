package wallylib

import (
	"errors"
	"fmt"
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

		if ind.ReceiverType != "" {
			if !fi.matchReceiver(ind.Package, ind.ReceiverType) {
				continue
			}
		}

		match = &ind
	}
	return match
}

func (fi *FuncInfo) matchReceiver(pkg, recvType string) bool {
	if fi.Signature == nil || fi.Signature.Recv() == nil {
		return false
	}

	recString := fmt.Sprintf("%s.%s", pkg, recvType)
	funcRecv := fi.Signature.Recv().Type().String()

	if recString == funcRecv || fmt.Sprintf("*%s", recString) == funcRecv {
		return true
	}
	return false
}

func GetFuncInfo(expr ast.Expr, info *types.Info) (*FuncInfo, error) {
	var funcIdent *ast.Ident
	var x ast.Expr

	switch funcExpr := expr.(type) {
	case *ast.Ident:
		funcIdent = funcExpr
	case *ast.SelectorExpr:
		funcIdent = funcExpr.Sel
		x = funcExpr.X
	case *ast.IndexExpr: // covers generics
		if idt, ok := funcExpr.X.(*ast.Ident); ok {
			funcIdent = idt
		} else {
			return nil, errors.New("unable to get func data")
		}
	default:
		return nil, errors.New("unable to get func data")
	}

	funcName := GetName(funcIdent)
	pkgPath, err := ResolvePackageFromIdent(funcIdent, info)
	if err != nil {
		if funcName != "" && x != nil {
			// Try to get pkg name from the selector, as this is likely not a pkg.func
			// but a struct.fun
			pkgPath, err = ResolvePackageFromIdent(x, info)
			if err != nil {
				return nil, err
			}
		} else {
			return nil, errors.New("unable to get func data")
		}
	}

	// TODO: maybe worth returning an error if we cannot get the signature, as we don't support
	// anonymous functions and closures as targetted functions via indicators anyway
	sig, _ := GetFuncSignature(funcIdent, info)

	return &FuncInfo{
		Package: pkgPath.Path(),
		Pkg:     pkgPath,
		//Type: nil,
		Name:      funcName,
		Signature: sig,
	}, nil
}

func GetFuncSignature(expr ast.Expr, info *types.Info) (*types.Signature, error) {
	switch expr := expr.(type) {
	case *ast.Ident:
		obj := info.ObjectOf(expr)
		return getSignatureFromObject(obj)
	case *ast.CallExpr:
		if ident, ok := expr.Fun.(*ast.Ident); ok {
			obj := info.ObjectOf(ident)
			return getSignatureFromObject(obj)
		}
	}

	return nil, errors.New("unable to get signature from expression")
}

func getSignatureFromObject(obj types.Object) (*types.Signature, error) {
	switch obj := obj.(type) {
	case *types.Func:
		return obj.Type().(*types.Signature), nil
	case *types.Var:
		if sig, ok := obj.Type().(*types.Signature); ok {
			return sig, nil
		}
	}
	return nil, errors.New("object is not a function or does not have a signature")
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
		// This loop makes sure we obtain CEs when in the body function literal used
		// as arguments to CEs. See https://github.com/hashicorp/nomad/blob/d34788896f8892377a9039b81a65abd7a913b3cc/nomad/csi_endpoint.go#L1633
		// for an example
		for _, v := range e.Args {
			if rr, ok := v.(*ast.FuncLit); ok {
				return GetExprsFromStmt(rr.Body)
			}
		}
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
