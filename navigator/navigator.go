package navigator

import (
	"fmt"
	"go/ast"
	"go/token"
	"go/types"
	"golang.org/x/tools/go/analysis"
	"golang.org/x/tools/go/analysis/passes/inspect"
	"golang.org/x/tools/go/ast/inspector"
	"golang.org/x/tools/go/packages"
	"log/slog"
	"os"
	"wally/checker"
	"wally/indicator"
	"wally/logger"
	"wally/reporter"
	"wally/wallylib"
)

type Navigator struct {
	Logger          *slog.Logger
	RouteIndicators []indicator.Indicator
	RouteMatches    []wallylib.RouteMatch
}

func NewNavigator(logLevel int, indicators []indicator.Indicator) *Navigator {
	return &Navigator{
		Logger:          logger.NewLogger(logLevel),
		RouteIndicators: indicators,
	}
}

func (n *Navigator) MapRoutes(path string) {
	if path == "" {
		path = "./..."
	}

	pkgs := LoadPackages(path)

	var analyzer = &analysis.Analyzer{
		Name:     "wally",
		Doc:      "maps HTTP and RPC routes",
		Run:      n.Run,
		Requires: []*analysis.Analyzer{inspect.Analyzer},
	}

	checker := checker.InitChecker(analyzer)
	// TODO: consider this as part of a checker instead
	results := map[*analysis.Analyzer]interface{}{}
	for _, pkg := range pkgs {
		pass := &analysis.Pass{
			Analyzer:          checker.Analyzer,
			Fset:              pkg.Fset,
			Files:             pkg.Syntax,
			OtherFiles:        pkg.OtherFiles,
			IgnoredFiles:      pkg.IgnoredFiles,
			Pkg:               pkg.Types,
			TypesInfo:         pkg.TypesInfo,
			TypesSizes:        pkg.TypesSizes,
			ResultOf:          results,
			Report:            func(d analysis.Diagnostic) {},
			ImportObjectFact:  checker.ImportObjectFact,
			ExportObjectFact:  checker.ExportObjectFact,
			ImportPackageFact: nil,
			ExportPackageFact: nil,
			AllObjectFacts:    nil,
			AllPackageFacts:   nil,
		}

		res, err := inspect.Analyzer.Run(pass)
		if err != nil {
			fmt.Printf(err.Error())
			continue
		}

		pass.ResultOf[inspect.Analyzer] = res

		result, err := pass.Analyzer.Run(pass)
		if err != nil {
			n.Logger.Warn("Error running analyzer %s: %s\n", checker.Analyzer.Name, err)
			continue
		}
		// This should be placed outside of this loop
		// we want to collect single results here, then run through all at the end.
		if result != nil {
			//fmt.Println("printing results")
			if passIssues, ok := result.([]wallylib.RouteMatch); ok {
				for _, iss := range passIssues {
					n.RouteMatches = append(n.RouteMatches, iss)
				}
			}
		}
	}
}

func LoadPackages(path string) []*packages.Package {
	fset := token.NewFileSet()

	cfg := &packages.Config{
		Mode: packages.NeedFiles | packages.NeedSyntax | packages.NeedTypes | packages.NeedTypesInfo |
			packages.NeedName | packages.NeedCompiledGoFiles | packages.NeedImports |
			packages.NeedExportFile | packages.NeedTypesSizes | packages.NeedModule | packages.NeedDeps,
		Fset: fset,
	}

	pkgs, err := packages.Load(cfg, path)
	if err != nil {
		fmt.Fprintf(os.Stderr, "load: %v\n", err)
		os.Exit(1)
	}

	return pkgs
}

func (n *Navigator) Run(pass *analysis.Pass) (interface{}, error) {
	inspecting := pass.ResultOf[inspect.Analyzer].(*inspector.Inspector)

	nodeFilter := []ast.Node{
		(*ast.CallExpr)(nil),
		//(*ast.DeclStmt)(nil),
		(*ast.GenDecl)(nil),
		(*ast.AssignStmt)(nil),
	}

	results := []wallylib.RouteMatch{}

	// this is basically the same as ast.Inspect(), only we don't return a
	// boolean anymore as it'll visit all the nodes based on the filter.
	inspecting.Preorder(nodeFilter, func(node ast.Node) {

		if de, ok := node.(*ast.AssignStmt); ok {
			for _, exp := range de.Rhs {
				if id, ok := exp.(*ast.Ident); ok {
					res := wallylib.GetValueFromExp(id, pass)
					if res != "" {
						fmt.Println("HOLYCRAP ", res)
					}
				}
			}
			//if v.Tok != token.DEFINE {
			//	return
			//}
			//for _, exp := range v.Lhs {
			//	if id, ok := exp.(*ast.Ident); ok {
			//		check(id, "var", initialisms)
			//	}
			//}
		}
		if gen, ok := node.(*ast.GenDecl); ok {
			for _, spec := range gen.Specs {
				switch s := spec.(type) {
				case *ast.ValueSpec:
					for k, id := range s.Values {
						res := wallylib.GetValueFromExp(id, pass)
						if res != "" {
							fmt.Println("FOUND SOMETHING! ", res, s.Names[k].Name)

							o1 := pass.TypesInfo.ObjectOf(s.Names[k])
							switch tt := o1.(type) {
							case *types.Var:
								fmt.Println("is a Var ", res, tt.Name())
								if tt.Parent() == tt.Pkg().Scope() {
									// Scope level
									fmt.Println("==================wowavar==================")
									//fmt.Println("is a pkg scope ", res, tt.Name())
									fmt.Println("wowavar Val", res)
									fmt.Println("wowavar name", s.Names[k].Name)
									fmt.Println("wowavar tt name", tt.Name())

									gv := new(checker.GlobalVar)
									gv.Val = res
									pass.ExportObjectFact(o1, gv)
								}
							case *types.Const:
								fmt.Println("is a Const ", res, tt.Name())
							}
							//fmt.Println("OFID ", s.Names[k].Name)
						}
					}
				}
			}
		}

		ce, ok := node.(*ast.CallExpr)
		if !ok {
			return
		}
		// We have a function if we have made it here
		funExpr := ce.Fun
		funcInfo, err := wallylib.GetFuncInfo(funExpr, pass.TypesInfo)
		if err != nil {
			return
		}

		route := funcInfo.Match(n.RouteIndicators)
		if route == nil {
			// Don't keep going deeper in the node if there are no matches by now?
			return
		}

		// Get the position of the function in code
		pos := pass.Fset.Position(funExpr.Pos())

		// Whether we are able to get params or not we have a match
		match := wallylib.RouteMatch{
			Indicator: *route,
			Pos:       pos,
		}

		sel, _ := funExpr.(*ast.SelectorExpr)

		sig, _ := wallylib.GetFuncSignature(sel.Sel, pass.TypesInfo)
		n.Logger.Debug("Checking for pos", "pos", pos.String())
		// Now try to get the params for methods, path, etc.
		match.Params = wallylib.ResolveParams(route.Params, sig, ce, pass)
		fmt.Println("match found ", match.Indicator.Function)
		results = append(results, match)
	})

	return results, nil
}

//func (n *Navigator) ParseFile(file *ast.File, pkg *packages.Package) {
//	ast.Inspect(file, func(node ast.Node) bool {
//		// If we are here, we need to keep looking until we find a function
//		ce, ok := node.(*ast.CallExpr)
//		// so we ask to keep digging in the node until we do
//		if !ok {
//			return true
//		}
//
//		// We have a function if we have made it here
//		funExpr := ce.Fun
//		funcInfo, err := wallylib.GetFuncInfo(funExpr, pkg.TypesInfo)
//		if err != nil {
//			return true
//		}
//
//		route := funcInfo.Match(n.RouteIndicators)
//		if route == nil {
//			// Don't keep going deeper in the node if there are no matches by now?
//			return true
//		}
//
//		// Get the position of the function in code
//		pos := pkg.Fset.Position(funExpr.Pos())
//
//		// Whether we are able to get params or not we have a match
//		match := wallylib.RouteMatch{
//			Indicator: *route,
//			Pos:       pos,
//		}
//
//		sel, ok := funExpr.(*ast.SelectorExpr)
//
//		sig, _ := wallylib.GetFuncSignature(sel.Sel, pkg.TypesInfo)
//		n.Logger.Debug("Checking for pos", "pos", pos.String())
//		// Now try to get the params for methods, path, etc.
//		match.Params = wallylib.ResolveParams(route.Params, sig, ce, pkg.TypesInfo)
//
//		n.RouteMatches = append(n.RouteMatches, match)
//
//		return true
//	})
//}

func (n *Navigator) PrintResults() {
	reporter.PrintResults(n.RouteMatches)
}
