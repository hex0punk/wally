package callermapper

import (
	"go/ast"
	"golang.org/x/tools/go/analysis"
	"golang.org/x/tools/go/analysis/passes/inspect"
	"golang.org/x/tools/go/ast/inspector"
	"reflect"
	"wally/checker/cefinder"
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
				switch s := b.(type) {
				case *ast.ExprStmt:
					ce := GetCE(s.X)
					if ce != nil {
						cf.CE[fd] = append(cf.CE[fd], ce)
					}
				case *ast.IfStmt:
					ce := GetCE(s.Cond)
					if ce != nil {
						cf.CE[fd] = append(cf.CE[fd], ce)
					}
					// Should be checking body recursively too
					switch x := s.Init.(type) {
					case *ast.ExprStmt:
						ce := GetCE(x.X)
						if ce != nil {
							cf.CE[fd] = append(cf.CE[fd], ce)
						}
					case *ast.IfStmt:
						ce := GetCE(x.Cond)
						if ce != nil {
							cf.CE[fd] = append(cf.CE[fd], ce)
						}
					case *ast.AssignStmt:
						for _, rhs := range x.Rhs {
							ce := GetCE(rhs)
							if ce != nil {
								cf.CE[fd] = append(cf.CE[fd], ce)
							}
						}
						for _, lhs := range x.Lhs {
							ce := GetCE(lhs)
							if ce != nil {
								cf.CE[fd] = append(cf.CE[fd], ce)
							}
						}
					}
				case *ast.AssignStmt:
					for _, rhs := range s.Rhs {
						ce := GetCE(rhs)
						if ce != nil {
							cf.CE[fd] = append(cf.CE[fd], ce)
						}
					}
					for _, lhs := range s.Lhs {
						ce := GetCE(lhs)
						if ce != nil {
							cf.CE[fd] = append(cf.CE[fd], ce)
						}
					}

				}
			}
		}
	})
	return cf, nil
}

func CheckStmt() {

}

func GetCE(e ast.Expr) *ast.CallExpr {
	switch e := e.(type) {
	case *ast.CallExpr:
		return e
	}
	return nil
}
