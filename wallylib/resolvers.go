package wallylib

import (
	"errors"
	"fmt"
	"go/ast"
	"go/types"
	"wally/indicator"
)

func ResolveParams(params []indicator.RouteParam, sig *types.Signature, ce *ast.CallExpr, info *types.Info) map[string]string {
	resolvedParams := make(map[string]string)
	for _, param := range params {
		val := ""
		if param.Name != "" && sig != nil {
			val = ResolveParamFromName(param.Name, sig, ce, info)
		} else {
			val = ResolveParamFromPos(param.Pos, ce, info)
		}
		resolvedParams[param.Name] = val
	}
	return resolvedParams
}

func ResolveParamFromPos(pos int, param *ast.CallExpr, info *types.Info) string {
	if param.Args != nil && len(param.Args) > 0 {
		arg := param.Args[pos]
		return GetValueFromExp(arg, info)
	}
	return ""
}

func ResolveParamFromName(name string, sig *types.Signature, param *ast.CallExpr, info *types.Info) string {
	// First get the pos for the arg
	pos, err := GetParamPos(sig, name)
	if err != nil {
		// we failed at getting param, return an empty string and be sad (for now)
		return ""
	}

	return ResolveParamFromPos(pos, param, info)
}

func GetParamPos(sig *types.Signature, paramName string) (int, error) {
	numParams := sig.Params().Len()
	for i := 0; i < numParams; i++ {
		param := sig.Params().At(i)
		if param.Name() == paramName {
			return i, nil
		}
	}
	return 0, errors.New("Unable to find param pos")
}

func GetValueFromExp(exp ast.Expr, info *types.Info) string {
	switch node := exp.(type) {
	case *ast.BasicLit: // i.e. "/thepath"
		fmt.Println("FOUND A BASICLIT")
		return node.Value
	case *ast.SelectorExpr: // i.e. "paths.User" where User is a constant
		// If its a constant its a selector and we can extract the value below
		fmt.Println("FOUND A SELEC")
		o1 := info.ObjectOf(node.Sel)
		// TODO: Write a func for this
		if con, ok := o1.(*types.Const); ok {
			return con.Val().String()
		}
		if con, ok := o1.(*types.Var); ok {
			// A non-constant value, best effort (without ssa navigator) is to
			// return the variable name
			return fmt.Sprintf("<var %s.%s>", GetName(node.X), con.Id())
		}
	case *ast.Ident: // i.e. user where user is a const
		fmt.Println("FOUND A IDENT")
		o1 := info.ObjectOf(node)
		// TODO: Write a func for this
		if con, ok := o1.(*types.Const); ok {
			return con.Val().String()
		}
	case *ast.CompositeLit: // i.e. []string{"POST"}
		fmt.Println("FOUND A COMPOSITE")
		vals := ""
		for _, lit := range node.Elts {
			val := GetValueFromExp(lit, info)
			vals = vals + " " + val
		}
		return vals
	case *ast.BinaryExpr: // i.e. base+"/getUser"
		fmt.Println("FOUND A BIN")
		left := GetValueFromExp(node.X, info)
		right := GetValueFromExp(node.Y, info)
		if left == "" {
			left = "<BinExp.X>"
		}
		if right == "" {
			right = "<BinExp.Y>"
		}
		// We assume the operator (be.Op) is +, because why would it be anything else
		// for a func param
		return left + right
	}
	fmt.Println("FOUND A NOTHING")
	return ""
}

// ResolvePackageFromIdent TODO: This may be useful to get receiver type of func
// Also, wrong name, its from an Expr, not from Idt, technically
func ResolvePackageFromIdent(expr ast.Expr, info *types.Info) (string, error) {
	idt, ok := expr.(*ast.Ident)
	if !ok {
		return "", errors.New("not an ident")
	}

	o1 := info.ObjectOf(idt)
	if o1 != nil && o1.Pkg() != nil {
		// TODO: Can also get the plain pkg name without path with `o1.Pkg().Name()`
		return o1.Pkg().Path(), nil
	}

	errStr := fmt.Sprintf("unable to get package name from Ident")
	return "", errors.New(errStr)
}
