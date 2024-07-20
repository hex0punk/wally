package navigator

import (
	"fmt"
	"go/ast"
	"go/token"
	"go/types"
	"golang.org/x/tools/go/analysis"
	"golang.org/x/tools/go/analysis/passes/ctrlflow"
	"golang.org/x/tools/go/analysis/passes/inspect"
	"golang.org/x/tools/go/ast/astutil"
	"golang.org/x/tools/go/ast/inspector"
	"golang.org/x/tools/go/callgraph"
	"golang.org/x/tools/go/callgraph/cha"
	"golang.org/x/tools/go/callgraph/rta"
	"golang.org/x/tools/go/callgraph/static"
	"golang.org/x/tools/go/callgraph/vta"
	"golang.org/x/tools/go/packages"
	"golang.org/x/tools/go/ssa"
	"golang.org/x/tools/go/ssa/ssautil"
	"log"
	"log/slog"
	"os"
	"wally/checker"
	"wally/indicator"
	"wally/logger"
	"wally/match"
	"wally/passes/callermapper"
	"wally/passes/cefinder"
	"wally/passes/tokenfile"
	"wally/reporter"
	"wally/wallylib"
	"wally/wallylib/callmapper"
)

type Navigator struct {
	Logger          *slog.Logger
	SSA             *SSA
	RouteIndicators []indicator.Indicator
	RouteMatches    []match.RouteMatch
	RunSSA          bool
	Packages        []*packages.Package
	CallgraphAlg    string
}

type SSA struct {
	Packages  []*ssa.Package
	Callgraph *callgraph.Graph
	Program   *ssa.Program
}

func NewNavigator(logLevel int, indicators []indicator.Indicator) *Navigator {
	return &Navigator{
		Logger:          logger.NewLogger(logLevel),
		RouteIndicators: indicators,
	}
}

// Copied from https://github.com/golang/tools/blob/master/cmd/callgraph/main.go#L291C1-L302C2
func mainPackages(pkgs []*ssa.Package) ([]*ssa.Package, error) {
	var mains []*ssa.Package
	for _, p := range pkgs {
		if p != nil && p.Pkg.Name() == "main" && p.Func("main") != nil {
			mains = append(mains, p)
		}
	}
	if len(mains) == 0 {
		return nil, fmt.Errorf("no main packages")
	}
	return mains, nil
}

func (n *Navigator) MapRoutes(paths []string) {
	if len(paths) == 0 {
		paths = append(paths, "./...")
	}

	pkgs := LoadPackages(paths)
	n.Packages = pkgs

	if n.RunSSA {
		n.Logger.Info("Building SSA program")
		n.SSA = &SSA{
			Packages: []*ssa.Package{},
		}
		prog, ssaPkgs := ssautil.AllPackages(pkgs, ssa.InstantiateGenerics)
		n.SSA.Packages = ssaPkgs
		n.SSA.Program = prog
		prog.Build()

		n.Logger.Info("Generating SSA based callgraph", "alg", n.CallgraphAlg)
		switch n.CallgraphAlg {
		case "static":
			n.SSA.Callgraph = static.CallGraph(prog)
		case "cha":
			n.SSA.Callgraph = cha.CallGraph(prog)
		case "rta":
			mains := ssautil.MainPackages(ssaPkgs)
			var roots []*ssa.Function
			for _, main := range mains {
				roots = append(roots, main.Func("init"), main.Func("main"))
			}
			rtares := rta.Analyze(roots, true)
			n.SSA.Callgraph = rtares.CallGraph
		case "vta":
			n.SSA.Callgraph = vta.CallGraph(ssautil.AllFunctions(prog), cha.CallGraph(prog))
		default:
			log.Fatalf("Unknown callgraph alg %s", n.CallgraphAlg)
		}
		n.Logger.Info("SSA callgraph generated successfully")
	}

	n.Logger.Info("Finding functions via AST parsing")
	// TODO: No real need to use ctrlflow.Analyzer if using SSA
	var analyzer = &analysis.Analyzer{
		Name:     "wally",
		Doc:      "maps HTTP and RPC routes",
		Run:      n.Run,
		Requires: []*analysis.Analyzer{inspect.Analyzer, ctrlflow.Analyzer, callermapper.Analyzer, tokenfile.Analyzer},
	}

	wallyChecker := checker.InitChecker(analyzer)
	// TODO: consider this as part of a checker instead
	results := map[*analysis.Analyzer]interface{}{}
	for _, pkg := range pkgs {
		pkg := pkg
		pass := &analysis.Pass{
			Analyzer:          wallyChecker.Analyzer,
			Fset:              pkg.Fset,
			Files:             pkg.Syntax,
			OtherFiles:        pkg.OtherFiles,
			IgnoredFiles:      pkg.IgnoredFiles,
			Pkg:               pkg.Types,
			TypesInfo:         pkg.TypesInfo,
			TypesSizes:        pkg.TypesSizes,
			ResultOf:          results,
			Report:            func(d analysis.Diagnostic) {},
			ImportObjectFact:  wallyChecker.ImportObjectFact,
			ExportObjectFact:  wallyChecker.ExportObjectFact,
			ImportPackageFact: nil,
			ExportPackageFact: nil,
			AllObjectFacts:    nil,
			AllPackageFacts:   nil,
		}

		for _, a := range analyzer.Requires {
			res, err := a.Run(pass)
			if err != nil {
				n.Logger.Error("Error running analyzer %s: %s\n", wallyChecker.Analyzer.Name, err)
				continue
			}
			pass.ResultOf[a] = res
		}

		result, err := pass.Analyzer.Run(pass)
		if err != nil {
			n.Logger.Error("Error running analyzer %s: %s\n", wallyChecker.Analyzer.Name, err)
			continue
		}
		// This should be placed outside of this loop
		// we want to collect single results here, then run through all at the end.
		if result != nil {
			if passIssues, ok := result.([]match.RouteMatch); ok {
				for _, iss := range passIssues {
					n.RouteMatches = append(n.RouteMatches, iss)
				}
			}
		}
	}
}

func LoadPackages(paths []string) []*packages.Package {
	fset := token.NewFileSet()

	cfg := &packages.Config{
		Mode: packages.NeedFiles | packages.NeedSyntax | packages.NeedTypes | packages.NeedTypesInfo |
			packages.NeedName | packages.NeedCompiledGoFiles | packages.NeedImports |
			packages.NeedExportFile | packages.NeedTypesSizes | packages.NeedModule | packages.NeedDeps,
		Fset: fset,
	}

	pkgs, err := packages.Load(cfg, paths...)
	if err != nil {
		fmt.Fprintf(os.Stderr, "load: %v\n", err)
		os.Exit(1)
	}

	return pkgs
}

func (n *Navigator) cacheVariables(node ast.Node, pass *analysis.Pass) {
	if genDcl, ok := node.(*ast.GenDecl); ok {
		n.RecordGlobals(genDcl, pass)
	}

	if dclStmt, ok := node.(*ast.DeclStmt); ok {
		if genDcl, ok := dclStmt.Decl.(*ast.GenDecl); ok {
			n.RecordGlobals(genDcl, pass)
		}
	}

	if stmt, ok := node.(*ast.AssignStmt); ok {
		n.RecordLocals(stmt, pass)
	}
}

func (n *Navigator) Run(pass *analysis.Pass) (interface{}, error) {
	inspecting := pass.ResultOf[inspect.Analyzer].(*inspector.Inspector)
	callMapper := pass.ResultOf[callermapper.Analyzer].(*cefinder.CeFinder)
	//flow := pass.ResultOf[ctrlflow.Analyzer].(*ctrlflow.CFGs)

	nodeFilter := []ast.Node{
		(*ast.CallExpr)(nil),
		(*ast.GenDecl)(nil),
		(*ast.AssignStmt)(nil),
		(*ast.DeclStmt)(nil),
	}

	var results []match.RouteMatch

	// this is basically the same as ast.Inspect(), only we don't return a
	// boolean anymore as it'll visit all the nodes based on the filter.
	inspecting.Preorder(nodeFilter, func(node ast.Node) {
		n.cacheVariables(node, pass)

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
		funcMatch := match.NewRouteMatch(*route, pos)

		funcMatch.Module = n.GetModuleName(funcInfo.Pkg)

		// Now try to get the params for methods, path, etc.
		funcMatch.Params = wallylib.ResolveParams(route.Params, funcInfo.Signature, ce, pass)

		//Get the enclosing func
		if n.RunSSA {
			ssapkg := n.SSAPkgFromTypesPackage(pass.Pkg)
			if ssapkg != nil {
				if ssaEnclosingFunc := GetEnclosingFuncWithSSA(pass, ce, ssapkg); ssaEnclosingFunc != nil {
					funcMatch.EnclosedBy = fmt.Sprintf("%s.%s", pass.Pkg.Name(), ssaEnclosingFunc.Name())
					funcMatch.SSA.EnclosedByFunc = ssaEnclosingFunc
					funcMatch.SSA.SSAInstruction = n.GetCallInstructionFromSSAFunc(ssaEnclosingFunc, ce)

					if funcMatch.SSA.SSAInstruction != nil {
						funcMatch.SSA.SSAFunc = n.GetFunctionFromCallInstruction(funcMatch.SSA.SSAInstruction)
					} else {
						n.Logger.Debug("unable to get SSA instruction for function", "function", ssaEnclosingFunc.Name())
					}
				}
			}
		} else {
			if decl := callMapper.EnclosingFunc(ce); decl != nil {
				funcMatch.EnclosedBy = fmt.Sprintf("%s.%s", pass.Pkg.Name(), decl.Name.String())
			}
		}

		results = append(results, funcMatch)
	})

	return results, nil
}

func (n *Navigator) GetCallInstructionFromSSAFunc(enclosingFunc *ssa.Function, expr *ast.CallExpr) ssa.CallInstruction {
	for _, block := range enclosingFunc.Blocks {
		for _, instr := range block.Instrs {
			if call, ok := instr.(ssa.CallInstruction); ok {
				if n.isMatchingCall(call, expr) {
					return call
				}
			}
		}
	}

	return nil
}

func (n *Navigator) isMatchingCall(call ssa.CallInstruction, expr *ast.CallExpr) bool {

	var cp token.Pos
	if call.Value() == nil {
		cp = call.Common().Value.Pos()
	} else {
		cp = call.Value().Call.Value.Pos()
	}

	// Check with Lparem works for non-static calls
	if cp == expr.Pos() || call.Pos() == expr.Lparen {
		return true
	}
	return false
}

func (n *Navigator) GetCalledFunctionUsingEnclosing(enclosingFunc *ssa.Function, ce *ast.CallExpr) *ssa.Function {
	for _, block := range enclosingFunc.Blocks {
		for _, instr := range block.Instrs {
			if call, ok := instr.(*ssa.Call); ok {
				if call.Call.Pos() == ce.Pos() {
					if callee := call.Call.StaticCallee(); callee != nil {
						return callee
					}
				}
			}
		}
	}

	return nil
}

func (n *Navigator) GetFunctionFromCallInstruction(callInstr ssa.CallInstruction) *ssa.Function {
	callCommon := callInstr.Common()
	if callCommon == nil {
		return nil
	}

	return callCommon.StaticCallee()
}

func (n *Navigator) SSAPkgFromTypesPackage(pkg *types.Package) *ssa.Package {
	for _, rpkg := range n.SSA.Packages {
		if rpkg != nil && rpkg.Pkg != nil {
			if rpkg.Pkg.String() == pkg.String() {
				return rpkg
			}
		}
	}
	return nil
}

// TODO: very slow function as it checks every node, one by one, and whether it has a path
// to any of the matches. At the moment, not used and only prints results for testing
func (n *Navigator) SolvePathsSlow() {
	for _, no := range n.SSA.Callgraph.Nodes {
		for _, routeMatch := range n.RouteMatches {
			edges := callgraph.PathSearch(no, func(node *callgraph.Node) bool {
				if node.Func != nil && node.Func == routeMatch.SSA.EnclosedByFunc {
					return true
				} else {
					return false
				}
			})
			for _, s := range edges {
				fmt.Println("PATH IS: ", s.String())
			}
		}
	}
}

func (n *Navigator) SolveCallPaths(options callmapper.Options) {
	for i, routeMatch := range n.RouteMatches {
		i, routeMatch := i, routeMatch
		if n.SSA.Callgraph.Nodes[routeMatch.SSA.EnclosedByFunc] == nil {
			continue
		}
		cm := callmapper.NewCallMapper(&routeMatch, n.SSA.Callgraph.Nodes, options)
		if options.SearchAlg == callmapper.Dfs {
			n.RouteMatches[i].SSA.CallPaths = cm.AllPathsDFS(n.SSA.Callgraph.Nodes[routeMatch.SSA.EnclosedByFunc])
		} else {
			n.RouteMatches[i].SSA.CallPaths = cm.AllPathsBFS(n.SSA.Callgraph.Nodes[routeMatch.SSA.EnclosedByFunc])
		}
	}
}

func (n *Navigator) RecordGlobals(gen *ast.GenDecl, pass *analysis.Pass) {
	for _, spec := range gen.Specs {
		s, ok := spec.(*ast.ValueSpec)
		if !ok {
			continue
		}

		for k, id := range s.Values {
			res := wallylib.GetValueFromExp(id, pass)
			if res == "" {
				continue
			}

			o1 := pass.TypesInfo.ObjectOf(s.Names[k])
			if tt, ok := o1.(*types.Var); ok {
				// If same scope level as pkg
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

func (n *Navigator) RecordLocals(gen *ast.AssignStmt, pass *analysis.Pass) {
	for idx, e := range gen.Rhs {
		idt, ok := gen.Lhs[idx].(*ast.Ident)
		if !ok {
			return
		}

		o1 := pass.TypesInfo.ObjectOf(idt)
		if !wallylib.IsLocal(o1) {
			return
		}

		res := wallylib.GetValueFromExp(e, pass)
		if res == "" || res == "\"\"" {
			return
		}

		var fact checker.LocalVar
		gv := new(checker.LocalVar)
		pass.ImportObjectFact(o1, &fact)

		if fact.Vals != nil {
			gv.Vals = fact.Vals
			gv.Vals = append(gv.Vals, res)
			pass.ExportObjectFact(o1, gv)

		} else {
			gv.Vals = append(gv.Vals, res)
			pass.ExportObjectFact(o1, gv)
		}
	}
}

func (n *Navigator) GetModuleName(typesPkg *types.Package) string {
	pkg := n.getPackagesPackageFromTypesPackage(typesPkg)
	// This will happen if the indicator given is for a standard library function
	// or if the project does not support modules. In such cases, for now, the user would have to specify a filter using the `-f` flag
	if pkg == nil {
		return ""
	}
	if pkg.Module != nil {
		return pkg.Module.Path
	}
	return ""
}

func (n *Navigator) getPackagesPackageFromTypesPackage(typesPkg *types.Package) *packages.Package {
	typesPkgPath := typesPkg.Path()
	for _, pkg := range n.Packages {
		if pkg.PkgPath == typesPkgPath {
			return pkg
		}
	}
	return nil
}

func GetObjFromCe(ce *ast.CallExpr, info *types.Info) types.Object {
	var funcObj types.Object

	switch fun := ce.Fun.(type) {
	case *ast.Ident:
		funcObj = info.ObjectOf(fun)
	case *ast.SelectorExpr:
		funcObj = info.ObjectOf(fun.Sel)
	default:
		return nil
	}

	return funcObj
}

func GetEnclosingFuncWithSSA(pass *analysis.Pass, ce *ast.CallExpr, ssaPkg *ssa.Package) *ssa.Function {
	currentFile := File(pass, ce.Fun.Pos())
	ref, _ := astutil.PathEnclosingInterval(currentFile, ce.Pos(), ce.Pos())
	return ssa.EnclosingFunction(ssaPkg, ref)
}

func File(pass *analysis.Pass, pos token.Pos) *ast.File {
	m := pass.ResultOf[tokenfile.Analyzer].(map[*token.File]*ast.File)
	return m[pass.Fset.File(pos)]
}

func (n *Navigator) PrintResults(format string, fileName string) {
	if format == "json" {
		if err := reporter.PrintJson(n.RouteMatches, fileName); err != nil {
			n.Logger.Error("Error printing to json", "error", err.Error())
		}
	} else if format == "csv" {
		if err := reporter.WriteCSVFile(n.RouteMatches, fileName); err != nil {
			n.Logger.Error("Error printing CSV", "error", err.Error())
		}
	} else {
		reporter.PrintResults(n.RouteMatches)
	}
}
