package wallylib

import (
	"errors"
	"fmt"
	"go/ast"
	"go/token"
	"go/types"
	"wally/indicator"
)

type FuncInfo struct {
	Package   string
	Type      string
	Name      string
	Route     string
	Signature *types.Signature

	Co string
}

type RouteMatch struct {
	Indicator indicator.Indicator // It should be FuncInfo instead
	Params    map[string]string
	Pos       token.Position
	Signature *types.Signature

	EnclosedBy string

	Lasso string
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

func RecFind(o1 types.Object, sc *types.Scope) string {
	//experiment
	var par *types.Scope
	if sc == nil {
		par = o1.Parent()
	} else {
		par = sc
	}

	if par == nil {
		return ""
	}

	//fmt.Println("continue")
	for _, na := range par.Names() {
		o := o1.Pkg().Scope().Lookup(na)
		if fff, ok := o.(*types.Func); ok {
			fmt.Println("LASSOFOUND ", fff.FullName())
			return fff.FullName()
		}
	}

	if par == o1.Pkg().Scope() {
		return ""
	}

	return RecFind(o1, par.Parent())
}

func GetFuncInfo(expr ast.Expr, info *types.Info) (*FuncInfo, error) {
	sel, ok := expr.(*ast.SelectorExpr)
	if !ok {
		return nil, errors.New("unable to get func data")
	}

	funcName := GetName(sel.Sel)
	pkgPath, co, err := ResolvePackageFromIdent(sel.Sel, info)
	if err != nil && funcName != "" {
		// Try to get pkg name from the selector, as hti si likely not a pkg.func
		// but a struct.fun
		pkgPath, co, err = ResolvePackageFromIdent(sel.X, info)
		if err != nil {
			return nil, err
		}
	}

	fmt.Println("HIJOLE", co)

	return &FuncInfo{
		Package: pkgPath,
		//Type: nil,
		Name: funcName,
		Co:   co,
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
