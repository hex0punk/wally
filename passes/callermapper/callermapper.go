package callermapper

import (
	"go/ast"
	"golang.org/x/tools/go/analysis"
	"golang.org/x/tools/go/analysis/passes/inspect"
	"golang.org/x/tools/go/ast/inspector"
	"reflect"
	"wally/passes/cefinder"
	match "wally/wallylib"
)

var Analyzer = &analysis.Analyzer{
	Name:             "callermapper",
	Doc:              "creates a mapping of func to ce's",
	Run:              run,
	RunDespiteErrors: true,
	ResultType:       reflect.TypeOf(new(cefinder.CeFinder)),
}

func run(pass *analysis.Pass) (interface{}, error) {
	inspecting := pass.ResultOf[inspect.Analyzer].(*inspector.Inspector)
	nodeFilter := []ast.Node{
		(*ast.FuncDecl)(nil),
	}

	cf := cefinder.New()
	inspecting.Preorder(nodeFilter, func(node ast.Node) {
		fd, ok := node.(*ast.FuncDecl)
		if !ok {
			return
		}

		if fd.Body != nil && fd.Body.List != nil {
			for _, b := range fd.Body.List {
				if ce := match.GetExprsFromStmt(b); ce != nil && len(ce) > 0 {
					cf.CE[fd] = append(cf.CE[fd], ce...)
				}
			}
		}
	})
	return cf, nil
}
