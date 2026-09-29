package wallylib

import (
	"errors"
	"fmt"
	"github.com/hex0punk/wally/checker"
	"github.com/hex0punk/wally/indicator"
	"go/ast"
	"go/types"
	"golang.org/x/tools/go/analysis"
)

// ResolvedArg is one positional argument of a matched call site: its
// compile-time value if GetValueFromExp could resolve one, and its source
// span so a UI can highlight exactly that argument's token range.
type ResolvedArg struct {
	Name    string `json:"name"`
	Value   string `json:"value"`
	Line    int    `json:"line"`
	Col     int    `json:"col"`
	EndLine int    `json:"endLine"`
	EndCol  int    `json:"endCol"`
}

// ResolveAllArgs resolves every positional argument of ce generically --
// not just ones an indicator config names via RouteParam (see
// ResolveParams) -- keyed by the matched function's own parameter name
// from sig. Unlike ResolveParams, this needs no indicator config at all,
// which is what wally live's ad-hoc pkg/func queries build (see
// navigator.QueryParams.indicator): they have no RouteParam list to
// declare interest in specific args ahead of time.
//
// An argument GetValueFromExp couldn't resolve at all (returns "") is
// omitted -- nothing useful to show. One it resolved to a best-effort
// "<var x.y>" label is kept, matching GetValueFromExp's own existing
// convention of surfacing that as better than nothing.
func ResolveAllArgs(sig *types.Signature, ce *ast.CallExpr, pass *analysis.Pass) []ResolvedArg {
	if ce == nil || len(ce.Args) == 0 {
		return nil
	}
	var out []ResolvedArg
	for i, arg := range ce.Args {
		val := GetValueFromExp(arg, pass)
		if val == "" {
			continue
		}
		start := pass.Fset.Position(arg.Pos())
		end := pass.Fset.Position(arg.End())
		out = append(out, ResolvedArg{
			Name:    paramNameForArgIndex(sig, i),
			Value:   val,
			Line:    start.Line,
			Col:     start.Column,
			EndLine: end.Line,
			EndCol:  end.Column,
		})
	}
	return out
}

// paramNameForArgIndex names the parameter a positional argument binds to,
// handling a variadic tail (every arg from the last named parameter
// onward binds to that same parameter). Returns "" if sig is nil or i is
// out of range for a non-variadic signature -- the caller still shows the
// resolved value, just without a name, matching how ResolveParams already
// tolerates an unresolvable name via GetParamPos's own TODO'd fallback.
func paramNameForArgIndex(sig *types.Signature, i int) string {
	if sig == nil {
		return ""
	}
	n := sig.Params().Len()
	if n == 0 {
		return ""
	}
	if sig.Variadic() && i >= n-1 {
		return sig.Params().At(n - 1).Name()
	}
	if i < n {
		return sig.Params().At(i).Name()
	}
	return ""
}

func ResolveParams(params []indicator.RouteParam, sig *types.Signature, ce *ast.CallExpr, pass *analysis.Pass) map[string]string {
	resolvedParams := make(map[string]string)
	for _, param := range params {
		param := param
		val := ""
		if param.Name != "" && sig != nil {
			val = ResolveParamFromName(param.Name, sig, ce, pass)
		} else {
			val = ResolveParamFromPos(param.Pos, ce, pass)
		}
		resolvedParams[param.Name] = val
	}
	return resolvedParams
}

func ResolveParamFromPos(pos int, param *ast.CallExpr, pass *analysis.Pass) string {
	if param.Args != nil && len(param.Args) > 0 {
		arg := param.Args[pos]
		return GetValueFromExp(arg, pass)
	}
	return ""
}

func ResolveParamFromName(name string, sig *types.Signature, param *ast.CallExpr, pass *analysis.Pass) string {
	// First get the pos for the arg
	pos, err := GetParamPos(sig, name)
	if err != nil {
		// we failed at getting param, return an empty string and be sad (for now)
		return ""
	}

	return ResolveParamFromPos(pos, param, pass)
}

func GetParamPos(sig *types.Signature, paramName string) (int, error) {
	numParams := sig.Params().Len()
	for i := 0; i < numParams; i++ {
		param := sig.Params().At(i)
		if param.Name() == paramName {
			return i, nil
		}
	}
	// TODO: not great to return 0
	return 0, errors.New("unable to find param pos")
}

func GetValueFromExp(exp ast.Expr, pass *analysis.Pass) string {
	// This should actually be called only AFTER we have checked facts to see if the value was already obtained,
	// otherwise this could be double work for nothing. That way we also don't need to pass a Pass to so many
	// funcs here, andinstead can stick to packages.TypesInfo
	info := pass.TypesInfo
	switch node := exp.(type) {
	case *ast.BasicLit: // i.e. "/thepath"
		return node.Value
	case *ast.SelectorExpr: // i.e. "paths.User" where User is a constant
		// If its a constant its a selector and we can extract the value below
		o1 := info.ObjectOf(node.Sel)
		// TODO: Write a func for this
		if con, ok := o1.(*types.Const); ok {
			return con.Val().String()
		}
		if con, ok := o1.(*types.Var); ok {
			// Check if global
			var fact checker.GlobalVar
			if pass.ImportObjectFact(o1, &fact) {
				return fact.Val
			}
			// A non-constant value, best effort (without ssa navigator) is to
			// return the variable name
			return fmt.Sprintf("<var %s.%s>", GetName(node.X), con.Id())
		}
	case *ast.Ident: // i.e. user where user is a const
		o1 := info.ObjectOf(node)
		// TODO: Write a func for this
		if con, ok := o1.(*types.Const); ok {
			return con.Val().String()
		}

		// Likely a local var
		if con, ok := o1.(*types.Var); ok {
			var fact checker.LocalVar
			if pass.ImportObjectFact(o1, &fact) {
				var result string
				for i, v := range fact.Vals {
					if i == len(fact.Vals)-1 {
						result += " " + v
						continue
					}
					result += v + " || "
				}
				return result
			}
			// A non-constant value, best effort (without ssa navigator) is to
			// return the variable name
			return fmt.Sprintf("<var %s.%s>", node.Name, con.Id())
		}
	case *ast.CompositeLit: // i.e. []string{"POST"}
		vals := ""
		for _, lit := range node.Elts {
			val := GetValueFromExp(lit, pass)
			vals = vals + " " + val
		}
		return vals
	case *ast.BinaryExpr: // i.e. base+"/getUser"
		left := GetValueFromExp(node.X, pass)
		right := GetValueFromExp(node.Y, pass)
		if left == "" {
			left = "<BinExp.X>"
		}
		if right == "" {
			right = "<BinExp.Y>"
		}
		// We assume the operator (be.Op) is +, because why would it be anything else
		// for a func param
		return left + right
	}
	return ""
}

// ResolvePackageFromIdent TODO: This may be useful to get receiver type of func
// Also, wrong name, its from an Expr, not from Idt, technically
func ResolvePackageFromIdent(expr ast.Expr, info *types.Info) (*types.Package, error) {
	idt, ok := expr.(*ast.Ident)
	if !ok {
		return nil, errors.New("not an ident")
	}

	o1 := info.ObjectOf(idt)
	if o1 != nil && o1.Pkg() != nil {
		// TODO: Can also get the plain pkg name without path with `o1.Pkg().Name()`
		return o1.Pkg(), nil
	}

	return nil, errors.New("unable to get package name from Ident")
}
