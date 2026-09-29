package wallylib

import (
	"go/ast"
	"go/importer"
	"go/parser"
	"go/token"
	"go/types"
	"strings"
	"testing"

	"golang.org/x/tools/go/analysis"
)

const resolveAllArgsTestSrc = `package testpkg

const Foo = "constant-value"

func helper() string { return "" }

func Target(a string, rest ...string) {}

func caller() {
	other := helper()
	Target("literal", Foo, other, "tail1", "tail2", helper())
}
`

// buildResolveAllArgsTestPass type-checks resolveAllArgsTestSrc for real
// (rather than hand-building a *types.Signature/ast.CallExpr) so the test
// exercises the actual go/types object kinds GetValueFromExp switches on --
// a *types.Const, a *types.Var with no fact available, and a literal --
// same as a real call site wally's own AST walk would see.
func buildResolveAllArgsTestPass(t *testing.T) (*analysis.Pass, *ast.CallExpr, *types.Signature) {
	t.Helper()

	fset := token.NewFileSet()
	f, err := parser.ParseFile(fset, "test.go", resolveAllArgsTestSrc, 0)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}

	info := &types.Info{
		Types: make(map[ast.Expr]types.TypeAndValue),
		Defs:  make(map[*ast.Ident]types.Object),
		Uses:  make(map[*ast.Ident]types.Object),
	}
	conf := types.Config{Importer: importer.Default()}
	pkg, err := conf.Check("testpkg", fset, []*ast.File{f}, info)
	if err != nil {
		t.Fatalf("typecheck: %v", err)
	}

	sig, ok := pkg.Scope().Lookup("Target").Type().(*types.Signature)
	if !ok {
		t.Fatal("Target is not a function")
	}

	var call *ast.CallExpr
	ast.Inspect(f, func(n ast.Node) bool {
		if ce, ok := n.(*ast.CallExpr); ok {
			if id, ok := ce.Fun.(*ast.Ident); ok && id.Name == "Target" {
				call = ce
			}
		}
		return true
	})
	if call == nil {
		t.Fatal("could not find the call to Target in the test source")
	}

	pass := &analysis.Pass{
		Fset:      fset,
		Files:     []*ast.File{f},
		Pkg:       pkg,
		TypesInfo: info,
		// No facts available -- exercises GetValueFromExp's best-effort
		// "<var x.y>" fallback for the unresolvable local var, rather than
		// panicking on a nil ImportObjectFact.
		ImportObjectFact: func(obj types.Object, fact analysis.Fact) bool { return false },
	}
	return pass, call, sig
}

func TestResolveAllArgs(t *testing.T) {
	pass, call, sig := buildResolveAllArgsTestPass(t)

	args := ResolveAllArgs(sig, call, pass)

	// Target(a string, rest ...string) called with 6 args: "literal", Foo,
	// other, "tail1", "tail2", helper(). helper()'s return value can't be
	// resolved at all (GetValueFromExp has no case for a nested call
	// expression) and must be omitted entirely -- so 5 resolved args, not 6.
	if len(args) != 5 {
		t.Fatalf("got %d resolved args, want 5 (helper() should be omitted): %+v", len(args), args)
	}

	var aCount, restCount int
	var foundConst, foundVar bool
	for _, a := range args {
		switch a.Name {
		case "a":
			aCount++
			if a.Value != `"literal"` {
				t.Fatalf(`got Value=%q for param "a", want "literal" (with quotes)`, a.Value)
			}
		case "rest":
			restCount++
		default:
			t.Fatalf("unexpected param name %q in %+v", a.Name, a)
		}
		if a.Value == `"constant-value"` {
			foundConst = true
		}
		if strings.HasPrefix(a.Value, "<var") {
			foundVar = true
		}
		if a.Line == 0 || a.Col == 0 || a.EndLine == 0 || a.EndCol == 0 {
			t.Fatalf("expected a non-zero source span, got %+v", a)
		}
	}

	if aCount != 1 {
		t.Fatalf("got %d args bound to \"a\", want 1", aCount)
	}
	// Every arg after the first binds to the variadic "rest" tail; helper()
	// was already excluded from args above, leaving 4: Foo, other, "tail1",
	// "tail2".
	if restCount != 4 {
		t.Fatalf("got %d args bound to \"rest\", want 4", restCount)
	}
	if !foundConst {
		t.Fatalf("expected Foo's resolved constant value among resolved args, got %+v", args)
	}
	if !foundVar {
		t.Fatalf("expected a best-effort \"<var ...>\" entry for the unresolvable local var, got %+v", args)
	}
}

func TestResolveAllArgs_NilCallExprReturnsNil(t *testing.T) {
	if got := ResolveAllArgs(nil, nil, nil); got != nil {
		t.Fatalf("expected nil for a nil call expr, got %+v", got)
	}
}

func TestResolveAllArgs_NoArgsReturnsNil(t *testing.T) {
	call := &ast.CallExpr{}
	if got := ResolveAllArgs(nil, call, nil); got != nil {
		t.Fatalf("expected nil for a call with no args, got %+v", got)
	}
}
