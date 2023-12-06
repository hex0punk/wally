package tokenfile

import (
	"go/ast"
	"go/token"
	"reflect"

	"golang.org/x/tools/go/analysis"
)

// code for this analyzeris taken directly from
// https://github.com/dominikh/go-tools/blob/5447921adabdc6be434408ab8911a62fed3e0e52/analysis/facts/tokenfile/token.go
var Analyzer = &analysis.Analyzer{
	Name: "tokenfileanalyzer",
	Doc:  "creates a mapping of *token.File to *ast.File",
	Run: func(pass *analysis.Pass) (interface{}, error) {
		m := map[*token.File]*ast.File{}
		for _, af := range pass.Files {
			tf := pass.Fset.File(af.Pos())
			m[tf] = af
		}
		return m, nil
	},
	RunDespiteErrors: true,
	ResultType:       reflect.TypeOf(map[*token.File]*ast.File{}),
}
