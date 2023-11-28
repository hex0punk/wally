package internal

import (
	"errors"
	"fmt"
	"go/ast"
	"go/token"
	"go/types"
	"golang.org/x/tools/go/packages"
	"log/slog"
	"os"
	"strings"
	"wally/indicator"
)

type FuncInfo struct {
	Package   string
	Type      string
	Name      string
	Route     string
	Signature *types.Signature
}

type RouteMatch struct {
	Indicator indicator.Indicator // It should be FuncInfo instead
	Params    map[string]string
	Pos       token.Position
	Signature *types.Signature
}

type Navigator struct {
	Logger          *slog.Logger
	RouteIndicators []indicator.Indicator
	RouteMatches    []RouteMatch
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

func NewNavigator(logLevel int, indicators []indicator.Indicator) *Navigator {
	return &Navigator{
		Logger:          NewLogger(logLevel),
		RouteIndicators: indicators,
	}
}

//func (n *Navigator) CheckRouterMatches

func (n *Navigator) MapRoutes() {
	pkgs := LoadPackages("./...")

	for _, pkg := range pkgs {
		n.ParseAST(pkg)
	}
}

func PackageMatches(pkgStr string, indicators []indicator.Indicator) bool {
	pkgStr = strings.Trim(pkgStr, "\"")
	for _, indicator := range indicators {
		if pkgStr == indicator.Package {
			return true
		}
	}
	return false
}

func (n *Navigator) ParseFile(file *ast.File, pkg *packages.Package) {
	ast.Inspect(file, func(node ast.Node) bool {
		// If we are here, we need to keep looking until we find a function
		ce, ok := node.(*ast.CallExpr)
		// so we ask to keep digging in the node until we do
		if !ok {
			return true
		}

		// We have a function if we have made it here
		funExpr := ce.Fun
		funcInfo, err := GetFuncInfo(funExpr, pkg.TypesInfo)
		if err != nil {
			return true
		}

		route := funcInfo.Match(n.RouteIndicators)
		if route == nil {
			// Don't keep going deeper in the node if there are no matches by now?
			return true
		}

		// Get the position of the function in code
		pos := pkg.Fset.Position(funExpr.Pos())

		// Whether we are able to get params or not we have a match
		match := RouteMatch{
			Indicator: *route,
			Pos:       pos,
		}

		sel, ok := funExpr.(*ast.SelectorExpr)

		sig, _ := GetFuncSignature(sel.Sel, pkg.TypesInfo)
		n.Logger.Debug("Checking for pos", "pos", pos.String())
		// Now try to get the params for methods, path, etc.
		match.Params = ResolveParams(route.Params, sig, ce, pkg.TypesInfo)

		n.RouteMatches = append(n.RouteMatches, match)

		return true
	})
}

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

func ResolveParamFromName(name string, sig *types.Signature, param *ast.CallExpr, info *types.Info) string {
	// First get the pos for the arg
	pos, err := GetParamPos(sig, name)
	if err != nil {
		// we failed at getting param, return an empty string and be sad (for now)
		return ""
	}

	return ResolveParamFromPos(pos, param, info)
}

func ResolveParamFromPos(pos int, param *ast.CallExpr, info *types.Info) string {
	if param.Args != nil && len(param.Args) > 0 {
		arg := param.Args[pos]
		return GetValueFromExp(arg, info)
	}
	return ""
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
			// A non-constant value, best effort (without ssa analysis) is to
			// return the variable name
			return fmt.Sprintf("<var %s>" + con.Id())
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

func (n *Navigator) ParseAST(pkg *packages.Package) {
	for _, f := range pkg.Syntax {
		n.ParseFile(f, pkg)
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

func GetFuncInfo(expr ast.Expr, info *types.Info) (*FuncInfo, error) {
	sel, ok := expr.(*ast.SelectorExpr)
	if !ok {
		return nil, errors.New("unable to get func data")
	}

	funcName := GetName(sel.Sel)
	pkgPath, err := ResolvePackageFromIdent(sel.Sel, info)
	if err != nil && funcName != "" {
		// Try to get pkg name from the selector, as hti si likely not a pkg.func
		// but a struct.fun
		pkgPath, err = ResolvePackageFromIdent(sel.X, info)
		if err != nil {
			return nil, err
		}
	}

	return &FuncInfo{
		Package: pkgPath,
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

func (n *Navigator) PrintResults() {
	for _, match := range n.RouteMatches {
		// TODO: This is printing the values from the indicator
		// That's fine, and it works but it should print values
		// from those captured during analysis, just in case
		fmt.Println("===========MATCH===============")
		fmt.Println("Package: ", match.Indicator.Package)
		fmt.Println("Function: ", match.Indicator.Function)
		fmt.Println("Params: ")
		for k, v := range match.Params {
			if v == "" {
				fmt.Printf("	%s: %s\n", k, "<could not resolve>")
			} else {
				fmt.Printf("	%s: %s\n", k, v)
			}
		}
		fmt.Printf("Position %s:%d\n", match.Pos.Filename, match.Pos.Line)
		fmt.Println()
	}
	fmt.Println("Total Results: ", len(n.RouteMatches))
}

func NewLogger(level int) *slog.Logger {
	verbosity := parseVerbosity(level)
	opts := &slog.HandlerOptions{Level: verbosity}

	logger := slog.New(slog.NewTextHandler(os.Stdout, opts))
	return logger
}

func parseVerbosity(verbosityFlag int) slog.Level {
	switch verbosityFlag {
	case 2:
		return slog.LevelInfo
	case 3:
		return slog.LevelDebug
	default:
		return slog.LevelError
	}
}
