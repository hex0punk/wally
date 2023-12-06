package callermapper

import (
	"go/ast"
	"golang.org/x/tools/go/analysis"
	"golang.org/x/tools/go/analysis/passes/inspect"
	"golang.org/x/tools/go/ast/inspector"
	"reflect"
	"wally/checker/cefinder"
)

var Analyzer = &analysis.Analyzer{
	Name:             "callermapper",
	Doc:              "creates a mapping of func to ce's",
	Run:              run,
	RunDespiteErrors: true,
	ResultType:       reflect.TypeOf(new(cefinder.CeFinder)),
}

func run(pass *analysis.Pass) (interface{}, error) {
	inspecting := pass.ResultOf[inspect.Analyzer].(*inspector.Inspector)
	nodeFilter := []ast.Node{
		(*ast.FuncDecl)(nil),
	}

	cf := cefinder.New()
	inspecting.Preorder(nodeFilter, func(node ast.Node) {
		fd, ok := node.(*ast.FuncDecl)
		if !ok {
			return
		}

		if fd.Body != nil && fd.Body.List != nil {

			for _, b := range fd.Body.List {
				if ce := getExprsFromStmt(b); ce != nil && len(ce) > 0 {
					cf.CE[fd] = append(cf.CE[fd], ce...)
				}
			}
		}
	})
	return cf, nil
}

// TODO: This should really be in wallylib
func getExprsFromStmt(stmt ast.Stmt) []*ast.CallExpr {
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
				bodyExps := getExprsFromStmt(stm)
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
			initCe := getExprsFromStmt(s.Init)
			if initCe != nil && len(initCe) > 0 {
				result = append(result, initCe...)
			}
		}
		if s.Else != nil {
			elseCe := getExprsFromStmt(s.Else)
			if elseCe != nil && len(elseCe) > 0 {
				result = append(result, elseCe...)
			}
		}
		ces := getExprsFromStmt(s.Body)
		if ces != nil && len(ces) > 0 {
			result = append(result, ces...)
		}
	case *ast.BlockStmt:
		for _, stm := range s.List {
			ce := getExprsFromStmt(stm)
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
