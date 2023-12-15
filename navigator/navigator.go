package navigator

import (
	"fmt"
	"go/ast"
	"go/token"
	"go/types"
	"golang.org/x/tools/go/analysis"
	"golang.org/x/tools/go/analysis/passes/buildssa"
	"golang.org/x/tools/go/analysis/passes/ctrlflow"
	"golang.org/x/tools/go/analysis/passes/inspect"
	"golang.org/x/tools/go/ast/astutil"
	"golang.org/x/tools/go/ast/inspector"
	"golang.org/x/tools/go/callgraph"
	"golang.org/x/tools/go/callgraph/cha"
	"golang.org/x/tools/go/packages"
	"golang.org/x/tools/go/ssa"
	"golang.org/x/tools/go/ssa/ssautil"
	"log/slog"
	"os"
	"strings"
	"wally/checker"
	"wally/indicator"
	"wally/logger"
	"wally/passes/callermapper"
	"wally/passes/tokenfile"
	"wally/reporter"
	"wally/wallylib"
)

type Navigator struct {
	Logger          *slog.Logger
	SSA             *SSA
	RouteIndicators []indicator.Indicator
	RouteMatches    []wallylib.RouteMatch
	RunSSA          bool
	Packages        []*packages.Package
}

type SSA struct {
	Packages  []*ssa.Package
	Callgraph *callgraph.Graph
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
	n.Packages = pkgs

	prog, ssaPkgs := ssautil.AllPackages(pkgs, 0)
	n.SSA.Packages = ssaPkgs
	prog.Build()
	n.SSA.Callgraph = cha.CallGraph(prog)

	// TODO: No real need to use ctrlflow.Analyzer if using SSA
	// TODO: Also, add call graph functionality using SSA
	var analyzer = &analysis.Analyzer{
		Name:     "wally",
		Doc:      "maps HTTP and RPC routes",
		Run:      n.Run,
		Requires: []*analysis.Analyzer{inspect.Analyzer, ctrlflow.Analyzer, callermapper.Analyzer, tokenfile.Analyzer},
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
	//var ssaBuild *buildssa.SSA
	//if n.RunSSA {
	//	ssaBuild = pass.ResultOf[buildssa.Analyzer].(*buildssa.SSA)
	//}
	inspecting := pass.ResultOf[inspect.Analyzer].(*inspector.Inspector)
	//callMapper := pass.ResultOf[callermapper.Analyzer].(*cefinder.CeFinder)
	//flow := pass.ResultOf[ctrlflow.Analyzer].(*ctrlflow.CFGs)

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
		pos1 := pass.Fset.Position(node.Pos())
		if strings.Contains(pos1.String(), "operator.go") {
			fmt.Println("JAMIE: ", pos1.String())
		}
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
		ssapkg := n.PkgToPkg(funcInfo.Pkg)
		if ssapkg != nil {
			if ssaFunc := GetEnclosingFuncWithSSATwo(pass, ce, ssapkg); ssaFunc != nil {
				match.EnclosedBy = fmt.Sprintf("%s.%s", ssaFunc.Pkg.String(), ssaFunc.Name())
				match.EnclosedByFunc = ssaFunc
				//path := ""
			}
		}
		//if n.RunSSA {
		//	// TODO: So this is very very likely possible without having to run the build ssa analyzer and instead
		//	// having our own ssa program and packages as done here. The only think would be to obtain the ssa package that
		//	// macthes the ast package, then we can use the same func below but no need to pass buildssa and instead
		//	// we can just pass the ssa.Package
		//	// That may or may not make the comparing for path finding more reliable
		//	// Thing is, that might be slowler for just finging the enclosing func if that is all we need from ssa.
		//	if ssaFunc := GetEnclosingFuncWithSSA(pass, ce, ssaBuild); ssaBuild != nil {
		//		match.EnclosedBy = fmt.Sprintf("%s.%s", ssaFunc.Pkg.String(), ssaFunc.Name())
		//		match.EnclosedByFunc = ssaFunc
		//		path := ""
		//		//res := static.CallGraph(ssaBuild.Pkg.Prog)
		//		//path := ""
		//		//for _, f1 := range res.Nodes[ssaFunc].In {
		//		//	// caller of enclosing
		//		//	pos2 := pass.Fset.Position(f1.Pos())
		//		//	path = path + " 1--> " + pos2.String()
		//		//	//for _, f2 := range f1.Caller.In {
		//		//	//	path = path + " --> " + f2.String()
		//		//	//}
		//		//
		//		//	// enclosing of caller
		//		//	pos2 = pass.Fset.Position(f1.Caller.Func.Pos())
		//		//	path = path + " 2--> " + pos2.String()
		//		//
		//		//	for _, f2 := range res.Nodes[f1.Caller.Func].In {
		//		//		pos2 = pass.Fset.Position(f2.Pos())
		//		//		path = path + " 3--> " + f2.String()
		//		//	}
		//		//
		//		//	//f1.Caller.In
		//		//	for _, f2 := range f1.Caller.In {
		//		//		pos2 = pass.Fset.Position(f2.Pos())
		//		//		path = path + " 4--> " + f2.String()
		//		//
		//		//		for _, f3 := range f2.Caller.In {
		//		//			//pos2 = pass.Fset.Position(f2.Pos())
		//		//			path = path + " 5--> " + f3.String()
		//		//		}
		//		//	}
		//
		//		//prog, _ := ssautil.AllPackages(n.Packages, 0)
		//		//fmt.Println("BUILDING")
		//		//prog.Build()
		//		//fmt.Println("GRAPHING")
		//		//res := static.CallGraph(prog)
		//
		//		//fmt.Println("checking connections for: ", ssaFunc.String())
		//		//for _, no := range n.Callgraph.Nodes {
		//		//	huh := callgraph.PathSearch(no, func(node *callgraph.Node) bool {
		//		//		if node.Func != nil && node.Func.String() == ssaFunc.String() {
		//		//			return true
		//		//		} else {
		//		//			return false
		//		//		}
		//		//	})
		//		//
		//		//	for _, s := range huh {
		//		//		fmt.Println("PATH IS: ", s.String())
		//		//	}
		//		//}
		//
		//		//fmt.Println("ITER")
		//		//for _, f1 := range res.Nodes[ssaFunc].In {
		//		//	// caller of enclosing
		//		//	pos2 := pass.Fset.Position(f1.Pos())
		//		//	path = path + " 1--> " + pos2.String()
		//		//	//for _, f2 := range f1.Caller.In {
		//		//	//	path = path + " --> " + f2.String()
		//		//	//}
		//		//
		//		//	// enclosing of caller
		//		//	pos2 = pass.Fset.Position(f1.Caller.Func.Pos())
		//		//	path = path + " 2--> " + pos2.String()
		//		//
		//		//	for _, f2 := range res.Nodes[f1.Caller.Func].In {
		//		//		pos2 = pass.Fset.Position(f2.Pos())
		//		//		path = path + " 3--> " + f2.String()
		//		//	}
		//		//
		//		//	//f1.Caller.In
		//		//	for _, f2 := range f1.Caller.In {
		//		//		pos2 = pass.Fset.Position(f2.Pos())
		//		//		path = path + " 4--> " + f2.String()
		//		//
		//		//		for _, f3 := range f2.Caller.In {
		//		//			//pos2 = pass.Fset.Position(f2.Pos())
		//		//			path = path + " 5--> " + f3.String()
		//		//		}
		//		//	}
		//		//}
		//		match.CallPaths = path
		//		//}
		//
		//		//for _, f1 := range res.Nodes[ssaFunc].In {
		//		//	fmt.Println("ANODE: ", f1.Description())
		//		//}
		//
		//		//prog, _ := ssautil.AllPackages(n.Packages, 0)
		//		//fmt.Println("BUILDING")
		//		//prog.Build()
		//		//fmt.Println("GRAPHING")
		//		//res := static.CallGraph(prog)
		//		//fmt.Println("ITER")
		//		//for _, f1 := range res.Nodes[ssaFunc].In {
		//		//	// caller of enclosing
		//		//	pos2 := pass.Fset.Position(f1.Pos())
		//		//	path = path + " 1--> " + pos2.String()
		//		//	//for _, f2 := range f1.Caller.In {
		//		//	//	path = path + " --> " + f2.String()
		//		//	//}
		//		//
		//		//	// enclosing of caller
		//		//	pos2 = pass.Fset.Position(f1.Caller.Func.Pos())
		//		//	path = path + " 2--> " + pos2.String()
		//		//
		//		//	for _, f2 := range res.Nodes[f1.Caller.Func].In {
		//		//		pos2 = pass.Fset.Position(f2.Pos())
		//		//		path = path + " 3--> " + f2.String()
		//		//	}
		//		//
		//		//	//f1.Caller.In
		//		//	for _, f2 := range f1.Caller.In {
		//		//		pos2 = pass.Fset.Position(f2.Pos())
		//		//		path = path + " 4--> " + f2.String()
		//		//
		//		//		for _, f3 := range f2.Caller.In {
		//		//			//pos2 = pass.Fset.Position(f2.Pos())
		//		//			path = path + " 5--> " + f3.String()
		//		//		}
		//		//	}
		//		//}
		//
		//		//for ff, _ := range callgraph.CalleesOf(res.CallGraph.Root) {
		//		//	path = path + " --> " + ff.String()
		//		//}
		//
		//		//for _, f1 := range res.CallGraph.Nodes {
		//		//	path = path + " --> " + f1.String()
		//		//}
		//
		//		//mains := ssautil.MainPackages([]*ssa.Package{res.CallGraph.Root.Func.Pkg})
		//		//if len(mains) > 0 {
		//		//	fmt.Println("FOUND MORE")
		//		//}
		//		//
		//		//initT := res.CallGraph.Root.Func.Pkg.Func("main")
		//		//if initT != nil {
		//		//	fmt.Println("HEREEEE!")
		//		//}
		//		//res2 := rta.Analyze([]*ssa.Function{initT}, true)
		//		//res3 := callgraph.PathSearch(res.CallGraph.Root, func(nn *callgraph.Node) bool {
		//		//	if nn.Func == initT {
		//		//		return true
		//		//	}
		//		//	return false
		//		//})
		//		//
		//		//for _, f1 := range res3 {
		//		//	path = path + " --> " + f1.String()
		//		//}
		//		//
		//		//for f1, _ := range res.CallGraph. {
		//		//	path = path + " --> " + f1.String()
		//		//}
		//
		//		//
		//		//if ssaFunc.Parent() != nil {
		//		//	path = ssaFunc.Parent().Name() + "canary"
		//		//}
		//		//
		//		//for _, fa := range ssaBuild.SrcFuncs {
		//		//	pos2 := pass.Fset.Position(fa.Pos())
		//		//	ob := fa.Object()
		//		//	switch y := ob.(type) {
		//		//	case *types.
		//		//	}
		//		//	path = path + " --> " + pos2.String()
		//	}
		//
		//	//			//fmt.Println("OHCRAP:
		//}

		//} else {
		//	if decl := callMapper.EnclosingFunc(ce); decl != nil {
		//		match.EnclosedBy = fmt.Sprintf("%s.%s", pass.Pkg.Name(), decl.Name.String())
		//
		//		if flow != nil {
		//			path := ""
		//			blocks := flow.FuncDecl(decl)
		//			for _, block := range blocks.Blocks {
		//				//for _, n2 := range block.Nodes {
		//				//	pos2 := pass.Fset.Position(n2.Pos())
		//				//	path = path + " --> " + pos2.String()
		//				//	if a, ok := n2.(*ast.FuncDecl); ok {
		//				//		path = path + " --> " + a.Name.String()
		//				//		fmt.Println("ADDEDA")
		//				//	}
		//				//}
		//
		//				for _, n2 := range block.Succs {
		//					for _, n3 := range n2.Nodes {
		//						pos2 := pass.Fset.Position(n3.Pos())
		//						path = path + " --> " + pos2.String()
		//						if a, ok := n3.(*ast.FuncDecl); ok {
		//							path = path + " --> " + a.Name.String()
		//							fmt.Println("ADDEDA")
		//						}
		//					}
		//				}
		//			}
		//			match.CallPaths = path
		//			//fmt.Println("OHCRAP: ", path)
		//		}
		//	}
		//}

		results = append(results, match)
	})

	return results, nil
}

func (n *Navigator) PkgToPkg(pkg *types.Package) *ssa.Package {
	for _, rpkg := range n.SSA.Packages {
		if rpkg.Pkg.String() == pkg.String() {
			return rpkg
		}
	}
	return nil
}

func (n *Navigator) SolvePaths(macthes []wallylib.RouteMatch) {
	//total := len(n.Callgraph.Nodes)
	for _, no := range n.SSA.Callgraph.Nodes {
		//if ct == (total / 4)
		//fmt.Println("checking connections for: ", ssaFunc.String())
		for _, match := range macthes {
			huh := callgraph.PathSearch(no, func(node *callgraph.Node) bool {
				if node.Func != nil && node.Func == match.EnclosedByFunc {
					return true
				} else {
					return false
				}
			})
			for _, s := range huh {
				fmt.Println("PATH IS: ", s.String())
			}
		}
	}
}

func (n *Navigator) SolvePathsTwo() {
	//total := len(n.Callgraph.Nodes)

	for i, match := range n.RouteMatches {
		fmt.Println("===================DESC FOR: ", match.EnclosedBy)
		match.SSA.Edges = n.SSA.Callgraph.Nodes[match.SSA.EnclosedByFunc].In

		allPaths := match.AllPaths(n.SSA.Callgraph.Nodes[match.SSA.EnclosedByFunc])
		fmt.Println("THERESIS", len(allPaths))
		for _, r := range allPaths {
			fmt.Println("---------THERESIS:", r)
			for _, r2 := range r {
				fmt.Println("*************THERESIS", r2)
			}
		}
		res := n.IterThing(n.SSA.Callgraph.Nodes[match.SSA.EnclosedByFunc].In, nil)
		//for _, no := range n.Callgraph.Nodes[match.EnclosedByFunc].In {
		//	fmt.Println("DESC: ", no.String())
		//	for _, no2 := range no.Caller.In {
		//		fmt.Println("DESC: ", no2.String())
		//	}
		//}
		n.RouteMatches[i].CallPaths = res
		fmt.Println()
	}
}

//	func (n *Navigator) AllPaths() {
//		visited := make(map[*callgraph.Node]bool)
//		paths := [][]string{}
//		path := []string{}
//	}
//
//	func (n *wallylib.RouteMatch) DFS(s *callgraph.Node, visited map[*Node]bool, path []string, paths *[][]string) {
//		visited[s] = true
//		path = append(path, s.name)
//
//		if s == nil || s.Func == nil {
//			*paths = append(*paths, path)
//		} else {
//			for _, e := range n.Callgraph.Nodes[match.EnclosedByFunc].In {
//				if e.source == s && !visited[e.dest] {
//					g.DFS(e.dest, d, visited, path, paths)
//				}
//			}
//		}
//
//		delete(visited, s)
//		path = path[:len(path)-1]
//	}
func (n *Navigator) IterThing(edges []*callgraph.Edge, res []string) [][]string {
	//total := len(n.Callgraph.Nodes)
	var total [][]string
	for _, no := range edges {
		//path := ""
		if no == nil {
			break
		}
		fmt.Printf("DESC: %s {{%s}}\n", no.Site.Parent().String(), no.String())
		res = append(res, no.Site.Parent().String())
		n.IterThing(no.Caller.In, res)
		total = append(total, res)
		//if i <= len(edges) {
		//	n.IterThing(no.Caller.In, res)
		//} else {
		//	total = append(total, res)
		//}

		//for _, i := range interPath {
		//	path = path + i
		//}
		//paths = append(paths, path)
		fmt.Println("-------------------------DESC END-----------------------")

		//for _, no2 := range no.Caller.In {
		//	fmt.Println("DESC: ", no2.String())
		//}
	}
	fmt.Println("-------------------------DESC BY BY-----------------------", len(total))
	for _, to := range total {
		fmt.Println("GOTTED", to[0])
	}
	return total
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

func GetEnclosingFuncWithSSATwo(pass *analysis.Pass, ce *ast.CallExpr, ssaPkg *ssa.Package) *ssa.Function {
	currentFile := File(pass, ce.Fun.Pos())
	ref, _ := astutil.PathEnclosingInterval(currentFile, ce.Pos(), ce.Pos())
	return ssa.EnclosingFunction(ssaPkg, ref)
}

func File(pass *analysis.Pass, pos token.Pos) *ast.File {
	m := pass.ResultOf[tokenfile.Analyzer].(map[*token.File]*ast.File)
	return m[pass.Fset.File(pos)]
}

func (n *Navigator) PrintResults() {
	reporter.PrintResults(n.RouteMatches)
}
