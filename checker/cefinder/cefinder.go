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
