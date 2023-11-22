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
)

type IndicatorType int

const (
	SERVICE IndicatorType = iota
	CALLER
)

type FuncInfo struct {
	Package   string
	Type      string
	Name      string
	Route     string
	Signature *types.Signature
}

type RouteIndicator struct {
	Package        string
	Type           string
	Function       string
	RouteParamPos  int
	RouteParamName string
	IndicatorType  IndicatorType
}

type RouteMatch struct {
	Indicator   RouteIndicator
	RouteString string
	Pos         token.Position
}

type Navigator struct {
	Logger          *slog.Logger
	RouteIndicators []RouteIndicator
	RouteMatches    []RouteMatch
}

func (fi *FuncInfo) Match(indicators []RouteIndicator) *RouteIndicator {
	var match *RouteIndicator
	for _, ind := range indicators {
		ind := ind
		if fi.Package != ind.Package {
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

func InitIndicators() []RouteIndicator {
	return []RouteIndicator{
		{
			Package:        "github.com/hashicorp/nomad/nomad",
			Type:           "",
			Function:       "forward",
			RouteParamName: "method",
			RouteParamPos:  0,
		},
	}
}

func NewNavigator(logLevel int, indicators []RouteIndicator) *Navigator {
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

func PackageMatches(pkgStr string, indicators []RouteIndicator) bool {
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
		funcInfo, err := n.GetFuncInfo(funExpr, pkg.TypesInfo)
		if err != nil {
			return true
		}

		res := funcInfo.Match(n.RouteIndicators)
		if res == nil {
			// Don't keep going deeper in the node if there are no matches by now?
			return true
		}

		// Now try to get the method value
		// TODO: move this to a dedicated function, or as part of GetFuncInfo
		// Also, there is too much code repetition here
		sel, ok := funExpr.(*ast.SelectorExpr)
		routeOrMethod := ""
		if ok {
			sig, err := n.GetFuncSignature(sel.Sel, pkg.TypesInfo)
			if err != nil {
				routeOrMethod = n.ResolveParamFromPos(res.RouteParamPos, ce, pkg.TypesInfo)
			} else {
				routeOrMethod = n.ResolveParamFromName(res.RouteParamName, sig, ce, pkg.TypesInfo)
			}
		} else {
			routeOrMethod = n.ResolveParamFromPos(res.RouteParamPos, ce, pkg.TypesInfo)
		}

		pos := pkg.Fset.Position(funExpr.Pos())
		match := RouteMatch{
			Indicator:   *res,
			RouteString: routeOrMethod,
			Pos:         pos,
		}

		n.RouteMatches = append(n.RouteMatches, match)
		return true
	})
}

func (n *Navigator) ResolveParamFromName(name string, sig *types.Signature, param *ast.CallExpr, info *types.Info) string {
	// First get the pos for the arg
	pos, err := n.GetParamPos(sig, info, name)
	if err != nil {
		// we failed at getting param, return an empty string and be sad (for now)
		return ""
	}

	return n.ResolveParamFromPos(pos, param, info)
}

func (n *Navigator) ResolveParamFromPos(pos int, param *ast.CallExpr, info *types.Info) string {
	// First get the pos for the arg
	//pos, err := n.GetParamPos(sig, info, name)
	//if err != nil {
	//	// we failed at getting param, return an empty string and be sad (for now)
	//	return ""
	//}
	if param.Args != nil && len(param.Args) > 0 {
		argMethod := param.Args[pos]

		// This is not enough if the value is a variable to a constant
		if _, ok := argMethod.(*ast.BasicLit); ok {
			return argMethod.(*ast.BasicLit).Value
		}

		// If its a constant its a selector and we can extract the value below
		if se, ok := argMethod.(*ast.SelectorExpr); ok {
			o1 := info.ObjectOf(se.Sel)
			// TODO: Write a func for this
			if con, ok := o1.(*types.Const); ok {
				return con.Val().String()
			}
		}
		if se, ok := argMethod.(*ast.Ident); ok {
			o1 := info.ObjectOf(se)
			// TODO: Write a func for this
			if con, ok := o1.(*types.Const); ok {
				return con.Val().String()
			}
		}
		if be, ok := argMethod.(*ast.BinaryExpr); ok {
			left := "BINEXPX"
			right := "BINEXPY"
			switch n := be.X.(type) {
			case *ast.BasicLit:
				left = n.Value
			case *ast.Ident:
				o1 := info.ObjectOf(n)
				// TODO: Write a func for this
				if con, ok := o1.(*types.Const); ok {
					left = con.Val().String()
				}
			}
			switch n := be.Y.(type) {
			case *ast.BasicLit:
				right = n.Value
			case *ast.Ident:
				o1 := info.ObjectOf(n)
				// TODO: Write a func for this
				if con, ok := o1.(*types.Const); ok {
					right = con.Val().String()
				}
			}
			// We assume the operator (be.Op) is +, because why would it be anything else
			return left + right
		}
	}
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

func (n *Navigator) GetFuncInfo(expr ast.Expr, info *types.Info) (*FuncInfo, error) {
	sel, ok := expr.(*ast.SelectorExpr)
	if !ok {
		return nil, errors.New("unable to get func data")
	}

	funcName := GetName(sel.Sel)
	pkgPath, err := n.ResolvePackageFromIdent(sel.Sel, info)
	if err != nil && funcName != "" {
		// Try to get pkg name from the selector, as hti si likely not a pkg.func
		// but a struct.fun
		pkgPath, err = n.ResolvePackageFromIdent(sel.X, info)
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

func (n *Navigator) GetFuncSignature(expr ast.Expr, info *types.Info) (*types.Signature, error) {
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
func (n *Navigator) ResolvePackageFromIdent(expr ast.Expr, info *types.Info) (string, error) {
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

func (n *Navigator) GetParamPos(sig *types.Signature, info *types.Info, paramName string) (int, error) {
	numParams := sig.Params().Len()
	for i := 0; i <= numParams; i++ {
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
		fmt.Println("Route", match.RouteString)
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
