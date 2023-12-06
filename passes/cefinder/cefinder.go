package cefinder

import "go/ast"

type CeFinder struct {
	CE map[*ast.FuncDecl][]*ast.CallExpr
}

func New() *CeFinder {
	return &CeFinder{
		CE: make(map[*ast.FuncDecl][]*ast.CallExpr),
	}
}

func (finder *CeFinder) EnclosingFunc(ce *ast.CallExpr) *ast.FuncDecl {
	for dec, fun := range finder.CE {
		for _, ex := range fun {
			if ex == ce.Fun || ex == ce {
				return dec
			}
		}
	}
	return nil
}
