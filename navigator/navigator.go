package navigator

import (
	"fmt"
	"github.com/hex0punk/wally/checker"
	"github.com/hex0punk/wally/indicator"
	"github.com/hex0punk/wally/logger"
	"github.com/hex0punk/wally/match"
	"github.com/hex0punk/wally/passes/callermapper"
	"github.com/hex0punk/wally/passes/cefinder"
	"github.com/hex0punk/wally/passes/tokenfile"
	"github.com/hex0punk/wally/reporter"
	"github.com/hex0punk/wally/wallylib"
	"github.com/hex0punk/wally/wallylib/callmapper"
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
	"strings"
	"sync"
	"time"
)

type Navigator struct {
	Logger          *slog.Logger
	SSA             *SSA
	RouteIndicators []indicator.Indicator
	RouteMatches    []match.RouteMatch
	RunSSA          bool
	Packages        []*packages.Package
	CallgraphAlg    string
	Exclusions      Exclusions
	// NoAutoDeps disables automatic expansion of the SSA build set to the
	// full same-module transitive closure of --paths (see
	// expandToModuleClosure). Only set this if you've deliberately scoped
	// --paths to include every package a call path might route through;
	// otherwise a call chain through an unlisted shared/internal package
	// (a helper library, a wrapper client, etc.) will silently look like a
	// dead end, since wally only builds real SSA function bodies (and thus
	// only traces call edges) for packages it was told to build.
	NoAutoDeps bool
}

type Exclusions struct {
	Packages    []string
	PosSuffixes []string
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

// MapRoutes loads the target packages, builds the SSA callgraph (if
// n.RunSSA), and finds all call sites matching n.RouteIndicators. This is
// the one-shot entry point used by the `map`/`map search` commands.
//
// It is equivalent to calling Build followed by FindMatches. Callers that
// need to issue more than one query against the same codebase (e.g. an
// interactive shell) should call Build once and FindMatches repeatedly
// instead: Build does the expensive part (package loading, type-checking,
// SSA construction, and callgraph generation), while FindMatches only
// re-walks the already-parsed ASTs looking for the current
// n.RouteIndicators, which is comparatively cheap.
func (n *Navigator) MapRoutes(paths []string) {
	n.Build(paths)
	n.FindMatches()
}

// Build loads the target packages and, if n.RunSSA, constructs the SSA
// program and its callgraph (per n.CallgraphAlg). This is the expensive,
// one-time setup step: package loading/type-checking and callgraph
// construction dominate wally's runtime on a large codebase. Callers that
// need to run more than one query against the same codebase should call
// Build once and then call FindMatches repeatedly.
func (n *Navigator) Build(paths []string) {
	if len(paths) == 0 {
		paths = append(paths, "./...")
	}

	pkgs := LoadPackages(paths)
	n.Packages = pkgs

	if n.RunSSA {
		ssaBuildPkgs := pkgs
		if !n.NoAutoDeps {
			expanded := expandToModuleClosure(pkgs)
			n.Logger.Info("Expanded SSA build set to same-module transitive closure", "roots", len(pkgs), "total", len(expanded))
			ssaBuildPkgs = expanded
		}

		n.Logger.Info("Building SSA program")
		n.SSA = &SSA{
			Packages: []*ssa.Package{},
		}
		prog, ssaPkgs := ssautil.AllPackages(ssaBuildPkgs, ssa.InstantiateGenerics)
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
}

// FindMatches walks the already-loaded packages' ASTs (see Build) looking
// for call sites matching n.RouteIndicators, appending to n.RouteMatches.
// Callers that want to issue a fresh query should reset n.RouteIndicators
// and n.RouteMatches before calling this again.
func (n *Navigator) FindMatches() {
	n.Logger.Info("Finding functions via AST parsing")
	pkgs := n.Packages
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
				n.RouteMatches = append(n.RouteMatches, passIssues...)
			}
		}
	}
}

// expandToModuleClosure walks the import graph of roots (the packages
// packages.Load returned for the given --paths) and returns roots plus
// every transitively-imported package that belongs to the same Go module(s)
// as the roots.
//
// This matters because ssautil.AllPackages only builds real SSA function
// bodies (the ones the callgraph can walk through) for the packages it's
// explicitly given; packages reachable only via .Imports are otherwise
// treated as external/bodyless. In practice, a target function is very
// often reached through a shared internal helper/wrapper package that isn't
// itself one of the cogs/services the caller thought to list in --paths
// (e.g. cogA calls target via some/shared/client, and --paths only named
// cogA) — without this expansion, that whole call chain silently looks like
// a dead end instead of surfacing an error.
//
// Deliberately stops at module boundaries: expanding into every transitive
// dependency including the standard library and third-party modules would
// make the SSA build set (and its memory cost) balloon for no benefit, since
// wally's targets are essentially always first-party code.
func expandToModuleClosure(roots []*packages.Package) []*packages.Package {
	rootModules := make(map[string]bool)
	for _, p := range roots {
		if p.Module != nil {
			rootModules[p.Module.Path] = true
		}
	}

	seen := make(map[*packages.Package]bool)
	var result []*packages.Package

	var visit func(p *packages.Package)
	visit = func(p *packages.Package) {
		if p == nil || seen[p] {
			return
		}
		seen[p] = true

		inRootModule := p.Module != nil && rootModules[p.Module.Path]
		if !inRootModule {
			return
		}
		result = append(result, p)

		for _, imp := range p.Imports {
			visit(imp)
		}
	}
	for _, p := range roots {
		visit(p)
	}
	return result
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

		// Get the position of the function in code
		pos := pass.Fset.Position(funExpr.Pos())
		if !n.PassesExclusions(pos, funcInfo.Package) {
			return
		}

		// This will be used for funcInfo.Match
		decl := callMapper.EnclosingFunc(ce)
		if decl != nil {
			funcInfo.EnclosedBy = &wallylib.FuncDecl{
				Pkg:  pass.Pkg,
				Decl: decl,
			}
		}

		route := funcInfo.Match(n.RouteIndicators)
		if route == nil {
			// Don't keep going deeper in the node if there are no matches by now?
			return
		}

		// Whether we are able to get params or not we have a match
		funcMatch := match.NewRouteMatch(*route, pos)

		if modName := n.GetModuleName(funcInfo.Pkg); modName != "" {
			funcMatch.Module = modName
		} else {
			funcMatch.Module = n.GetModuleName(funcInfo.EnclosedBy.Pkg)
		}

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
						funcMatch.SSA.SSAFunc = wallylib.GetFunctionFromCallInstruction(funcMatch.SSA.SSAInstruction)
					} else {
						n.Logger.Debug("unable to get SSA instruction for function", "function", ssaEnclosingFunc.Name())
					}
				}
			}
		}

		if funcMatch.EnclosedBy == "" {
			if decl != nil {
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

func (n *Navigator) PassesExclusions(pos token.Position, pkg string) bool {
	if len(n.Exclusions.Packages) == 0 && len(n.Exclusions.PosSuffixes) == 0 {
		return true
	}

	for _, pkg := range n.Exclusions.Packages {
		if pkg == pkg {
			return false
		}
	}

	for _, exc := range n.Exclusions.PosSuffixes {
		if strings.HasSuffix(pos.String(), exc) {
			return false
		}
	}
	return true
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
	var wg sync.WaitGroup

	for i, routeMatch := range n.RouteMatches {
		i, routeMatch := i, routeMatch

		if n.SSA.Callgraph.Nodes[routeMatch.SSA.EnclosedByFunc] == nil {
			continue
		}

		wg.Add(1)
		go func(i int, options callmapper.Options, routeMatch match.RouteMatch) {
			defer wg.Done()
			cm := callmapper.NewCallMapper(&routeMatch, n.SSA.Callgraph.Nodes, options)

			start := time.Now()
			n.Logger.Debug("Solving paths for match", "match", routeMatch.Pos.String())

			if options.SearchAlg == callmapper.Dfs {
				n.RouteMatches[i].SSA.CallPaths = cm.AllPathsDFS(n.SSA.Callgraph.Nodes[routeMatch.SSA.EnclosedByFunc])
			} else {
				n.RouteMatches[i].SSA.CallPaths = cm.AllPathsBFS(n.SSA.Callgraph.Nodes[routeMatch.SSA.EnclosedByFunc])
			}
			n.verifyCallPathImports(routeMatch, n.RouteMatches[i].SSA.CallPaths)

			duration := time.Since(start)
			n.Logger.Debug("Solved paths for match", "match", routeMatch.Pos.String(), "numPaths", len(n.RouteMatches[i].SSA.CallPaths.Paths), "duration", duration)
		}(i, options, routeMatch)
	}

	wg.Wait()
}

// verifyCallPathImports checks, hop by hop, whether each edge in every
// found call path is backed by a real, direct package import in the
// correct direction -- and records how deep from the target that holds
// (see CallPath.ConfirmedDepth). This exists because cha and vta can both
// resolve a call through a widely-implemented interface (the motivating
// case: grpc.ClientConnInterface.Invoke, satisfied by every generated gRPC
// client) by connecting call sites that share no real import relationship
// at all -- producing a path that looks real but is a callgraph
// over-approximation artifact.
//
// A per-hop check (rather than one aggregate "is there some transitive
// path" check on the outermost frame alone) is what makes this precise
// enough to handle both directions of a case that matters in practice: a
// genuinely fabricated single-hop dispatch (fails immediately, at hop 0)
// and a real multi-hop chain that happens to have generic shared
// framework/bootstrap code prepended ahead of its real entry point by a
// *different* instance of the same over-approximation (the inner hops
// confirm; only the outer, decorative ones don't). Trying to anchor on a
// single frame -- either the true outermost one, or a heuristically-chosen
// one -- can't distinguish these two shapes correctly at the same time; see
// the reverted attempt in this repo's history for a concrete case where a
// single-frame heuristic fixed one shape and broke the other.
func (n *Navigator) verifyCallPathImports(routeMatch match.RouteMatch, callPaths *match.CallPaths) {
	if callPaths == nil || routeMatch.SSA.EnclosedByFunc == nil {
		return
	}
	targetPkg := routeMatch.SSA.EnclosedByFunc.Package()
	if targetPkg == nil || targetPkg.Pkg == nil {
		return
	}
	targetPath := targetPkg.Pkg.Path()
	idx := wallylib.NewPackageIndex(n.Packages)

	for _, path := range callPaths.Paths {
		path.VerificationAttempted = true
		calleePath := targetPath
		depth := 0
		for _, node := range path.Nodes {
			caller := node.Caller
			if caller == nil || caller.Func == nil {
				// Can't resolve this hop's caller at all (shouldn't happen
				// in practice); fail open by treating it as confirmed
				// rather than truncating a path we can't actually assess.
				depth++
				calleePath = ""
				continue
			}
			callerPkg := caller.Func.Package()
			if callerPkg == nil || callerPkg.Pkg == nil {
				depth++
				calleePath = ""
				continue
			}
			callerPath := callerPkg.Pkg.Path()
			if !idx.DirectlyImports(callerPath, calleePath) {
				break
			}
			depth++
			calleePath = callerPath
		}
		path.ConfirmedDepth = depth
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
