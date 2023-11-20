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

type FuncInfo struct {
	Package string
	Type    string
	Name    string
	Route   string
}

type RouteIndicator struct {
	Package  string
	Type     string
	Function string
}

type RouteMatch struct {
	Indicator   RouteIndicator
	File        string
	RouteString string
	Pos         token.Position
}

type Navigator struct {
	Logger          *slog.Logger
	RouteIndicators []RouteIndicator
	RouteMatches    []RouteMatch
}

func (ri *FuncInfo) Match(indicators []RouteIndicator) bool {
	for _, ind := range indicators {
		if ri.Package != ind.Package {
			return false
		}
		if ri.Name != ind.Function {
			return false
		}
		if ri.Type != "" && ri.Type != ind.Type {
			return false
		}
	}
	return true
}

func InitIndicators() []RouteIndicator {
	return []RouteIndicator{
		{
			Package:  "github.com/hashicorp/nomad/nomad",
			Type:     "rpcHandler",
			Function: "forward",
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
		//fmt.Println("comparing " + pkgStr + " to " + indicator.Package)
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

		if !funcInfo.Match(n.RouteIndicators) {
			// Don't keep going deeper in the node if there are no matches by now?
			return true
		}

		// If GetFuncInfo didn't return a pkg name, then this is likely
		// not a package.func but a struct.func
		//pkgName := ""
		//if funcInfo.Package == "" {
		//	pkgName = ce.
		//}

		// Now get the route
		routeOrMethod := ResolveParam(0, ce, pkg.TypesInfo)

		pos := pkg.Fset.Position(funExpr.Pos())
		match := RouteMatch{
			Indicator:   n.RouteIndicators[0],
			RouteString: routeOrMethod,
			Pos:         pos,
		}

		n.RouteMatches = append(n.RouteMatches, match)
		return true
	})
}

func ResolveParam(pos int, ce *ast.CallExpr, info *types.Info) string {
	if ce.Args != nil && len(ce.Args) > 0 {
		argMethod := ce.Args[pos]
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

	resPkg := ""
	resPkg = GetName(sel.X)
	funcName := GetName(sel.Sel)
	if resPkg == "" && funcName != "" {
		// Try to get pkg name from the selector, as hti si likely not a pkg.func
		// but a struct.fun
		pkgPath, err := n.ResolvePackageFromIdent(sel.Sel, info)
		if err != nil {
			return nil, err
		}
		// TODO: ugly
		resPkg = pkgPath
	}

	return &FuncInfo{
		Package: resPkg,
		//Type: nil,
		Name: funcName,
	}, nil
}

// ResolvePackageFromIdent TODO: This may be useful to get receiver type of func
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
	//if f, ok := o1.(*types.Func); ok {
	//	sig := f.Type().(*types.Signature)
	//	if sig.Recv() != nil && sig.Recv().Type() != nil {
	//		n.Logger.Debug("Found func Params RECVTYPE: " + sig.Recv().Type().String())
	//	}
	//}
	//switch va := o1.(type) {
	//case *types.Func:
	//	if va.Pkg() != nil {
	//		return va.Pkg().Path(), nil
	//	}
	//}

	errStr := fmt.Sprintf("unable to get package name from Ident")
	return "", errors.New(errStr)
}

func (n *Navigator) PrintResults() {
	for _, match := range n.RouteMatches {
		fmt.Println("===========MATCH===============", match.File)
		fmt.Println("File: ", match.File)
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
