package navigator

import (
	"fmt"
	"go/ast"
	"go/token"
	"go/types"
	"golang.org/x/tools/go/analysis"
	"golang.org/x/tools/go/analysis/passes/buildssa"
	"golang.org/x/tools/go/analysis/passes/inspect"
	"golang.org/x/tools/go/ast/astutil"
	"golang.org/x/tools/go/ast/inspector"
	"golang.org/x/tools/go/packages"
	"golang.org/x/tools/go/ssa"
	"log/slog"
	"os"
	"wally/checker"
	"wally/indicator"
	"wally/logger"
	"wally/passes/callermapper"
	"wally/passes/cefinder"
	"wally/passes/tokenfile"
	"wally/reporter"
	"wally/wallylib"
)

type Navigator struct {
	Logger          *slog.Logger
	RouteIndicators []indicator.Indicator
	RouteMatches    []wallylib.RouteMatch
	RunSSA          bool
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
		Requires: []*analysis.Analyzer{inspect.Analyzer, callermapper.Analyzer, tokenfile.Analyzer},
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

		if n.RunSSA {
			res, err := buildssa.Analyzer.Run(pass)
			if err != nil {
				n.Logger.Error("Error running analyzer %s: %s\n", checker.Analyzer.Name, err)
				continue
			}
			pass.ResultOf[buildssa.Analyzer] = res
		}

		for _, a := range analyzer.Requires {
			res, err := a.Run(pass)
			if err != nil {
				n.Logger.Error("Error running analyzer %s: %s\n", checker.Analyzer.Name, err)
				continue
			}
			pass.ResultOf[a] = res
		}

		result, err := pass.Analyzer.Run(pass)
		if err != nil {
			n.Logger.Error("Error running analyzer %s: %s\n", checker.Analyzer.Name, err)
			continue
		}
		// This should be placed outside of this loop
		// we want to collect single results here, then run through all at the end.
		if result != nil {
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
	var ssaBuild *buildssa.SSA
	if n.RunSSA {
		ssaBuild = pass.ResultOf[buildssa.Analyzer].(*buildssa.SSA)
	}
	inspecting := pass.ResultOf[inspect.Analyzer].(*inspector.Inspector)
	callMapper := pass.ResultOf[callermapper.Analyzer].(*cefinder.CeFinder)

	nodeFilter := []ast.Node{
		(*ast.CallExpr)(nil),
		(*ast.GenDecl)(nil),
	}

	results := []wallylib.RouteMatch{}

	// this is basically the same as ast.Inspect(), only we don't return a
	// boolean anymore as it'll visit all the nodes based on the filter.
	inspecting.Preorder(nodeFilter, func(node ast.Node) {
		if gen, ok := node.(*ast.GenDecl); ok {
			n.RecordGlobals(gen, pass)
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

		//Get the enclosing func
		if n.RunSSA {
			if ssaFunc := GetEnclosingFuncWithSSA(pass, ce, ssaBuild); ssaBuild != nil {
				match.EnclosedBy = ssaFunc.Name()
			}
		} else {
			if decl := callMapper.EnclosingFunc(ce); decl != nil {
				match.EnclosedBy = decl.Name.String()
			}
		}

		results = append(results, match)
	})

	return results, nil
}

func (n *Navigator) RecordGlobals(gen *ast.GenDecl, pass *analysis.Pass) {
	for _, spec := range gen.Specs {
		s, ok := spec.(*ast.ValueSpec)
		if !ok {
			continue
		}
		for k, id := range s.Values {
			res := wallylib.GetValueFromExp(id, pass)
			if res != "" {
				continue
			}
			o1 := pass.TypesInfo.ObjectOf(s.Names[k])
			if tt, ok := o1.(*types.Var); ok {
				if tt.Parent() == tt.Pkg().Scope() {
					// Scope level
					gv := new(checker.GlobalVar)
					gv.Val = res
					pass.ExportObjectFact(o1, gv)
				}
			}
		}
	}
}

func GetEnclosingFuncWithSSA(pass *analysis.Pass, ce *ast.CallExpr, ssaBuild *buildssa.SSA) *ssa.Function {
	currentFile := File(pass, ce.Fun.Pos())
	ref, _ := astutil.PathEnclosingInterval(currentFile, ce.Pos(), ce.Pos())
	return ssa.EnclosingFunction(ssaBuild.Pkg, ref)
}

func File(pass *analysis.Pass, pos token.Pos) *ast.File {
	m := pass.ResultOf[tokenfile.Analyzer].(map[*token.File]*ast.File)
	return m[pass.Fset.File(pos)]
}

func (n *Navigator) PrintResults() {
	reporter.PrintResults(n.RouteMatches)
}
