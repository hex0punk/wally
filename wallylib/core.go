package wallylib

import (
	"errors"
	"fmt"
	"github.com/hex0punk/wally/indicator"
	"go/ast"
	"go/build"
	"go/types"
	"golang.org/x/tools/go/callgraph"
	"golang.org/x/tools/go/packages"
	"golang.org/x/tools/go/ssa"
	"strings"
)

type FuncDecl struct {
	Pkg  *types.Package
	Decl *ast.FuncDecl
}

func (f *FuncDecl) String() string {
	return fmt.Sprintf("%s.%s", f.Pkg.Name(), f.Decl.Name.String())
}

type FuncInfo struct {
	Package    string
	Pkg        *types.Package
	Type       string
	Name       string
	Route      string
	Signature  *types.Signature
	EnclosedBy *FuncDecl
}

type SSAContext struct {
	EnclosedByFunc *ssa.Function
	Edges          []*callgraph.Edge
	CallPaths      [][]string
}

// Match checks fi against each indicator, returning the first one that
// matches (nil if none do). idx, if non-nil, is used to resolve interface
// satisfaction for indicators that specify a ReceiverType -- see
// matchReceiver for why this matters and pkgIndex.go for what it costs to
// build. Passing nil idx preserves the exact-match-only behavior from
// before that resolution existed (still correct, just less capable for
// the interface-satisfied-by-a-type-in-a-different-package case).
func (fi *FuncInfo) Match(indicators []indicator.Indicator, idx *PackageIndex) *indicator.Indicator {
	var match *indicator.Indicator

	for _, ind := range indicators {
		ind := ind

		if fi.Name != ind.Function {
			continue
		}

		if ind.ReceiverType != "" {
			// matchReceiver checks receiver identity/satisfaction against
			// (ind.Package, ind.ReceiverType) directly -- a package-equality
			// check here would incorrectly reject a call dispatched through
			// an interface declared in a different package than the
			// concrete type the caller actually cares about (the case
			// matchReceiver's interface-satisfaction fallback exists for),
			// so it's intentionally skipped in this branch.
			if !fi.matchReceiver(ind.Package, ind.ReceiverType, idx) {
				continue
			}
		} else {
			// No ReceiverType given: package equality is the only signal
			// available, so keep requiring it -- this is the plain
			// --func/--pkg case with no receiver, unaffected by the above.
			if fi.Package != ind.Package && ind.Package != "*" {
				continue
			}
		}

		filterMatch := false
		if len(ind.MatchFilters) > 0 {
			for _, mf := range ind.MatchFilters {
				if mf != "" && fi.EnclosedBy.Pkg != nil {
					if strings.HasPrefix(fi.EnclosedBy.Pkg.Path(), mf) {
						filterMatch = true
						break
					}
				}
			}
			if !filterMatch {
				continue
			}
		}

		match = &ind
	}
	return match
}

// matchReceiver reports whether fi's call-site receiver type matches
// (pkg, recvType) -- either exactly (the original, still-primary check),
// or, failing that, via interface satisfaction if idx is available.
//
// The exact-match check alone misses a real, common shape: a call
// dispatched through an interface declared in a completely different
// package than the concrete type the caller is asking about (a
// caller-local interface satisfied by, but never referencing, a target's
// own type). fi.Signature.Recv().Type() in that case is the *caller's*
// interface -- its own package and name have no relationship to (pkg,
// recvType) at all, even though the concrete type the caller actually
// dispatches to at runtime does implement it. This can't be detected by
// string comparison; it needs an actual types.Implements check against
// the resolved (pkg, recvType) type, which only works if idx was built
// from a program that has that type loaded (idx == nil, e.g. no --ssa,
// falls back to exact-match-only).
func (fi *FuncInfo) matchReceiver(pkg, recvType string, idx *PackageIndex) bool {
	if fi.Signature == nil || fi.Signature.Recv() == nil {
		return false
	}

	recvT := fi.Signature.Recv().Type()
	recString := fmt.Sprintf("%s.%s", pkg, recvType)
	funcRecv := recvT.String()

	if recString == funcRecv || fmt.Sprintf("*%s", recString) == funcRecv {
		return true
	}

	if idx == nil {
		return false
	}
	iface, ok := recvT.Underlying().(*types.Interface)
	if !ok {
		// The call site's own receiver isn't an interface -- there's no
		// satisfaction question to ask, the exact-match check above was
		// already the right (and only) test, and it already failed.
		return false
	}
	target := idx.ResolveNamedType(pkg, recvType)
	if target == nil {
		return false
	}
	return types.Implements(target, iface) || types.Implements(types.NewPointer(target), iface)
}

func GetFuncInfo(expr ast.Expr, info *types.Info) (*FuncInfo, error) {
	var funcIdent *ast.Ident
	var x ast.Expr

	switch funcExpr := expr.(type) {
	case *ast.Ident:
		funcIdent = funcExpr
	case *ast.SelectorExpr:
		funcIdent = funcExpr.Sel
		x = funcExpr.X
	default:
		return nil, errors.New("unable to get func data")
	}

	funcName := GetName(funcIdent)
	pkgPath, err := ResolvePackageFromIdent(funcIdent, info)
	if err != nil {
		if funcName != "" && x != nil {
			// Try to get pkg name from the selector, as this is likely not a pkg.func
			// but a struct.fun
			pkgPath, err = ResolvePackageFromIdent(x, info)
			if err != nil {
				return nil, err
			}
		} else {
			return nil, errors.New("unable to get func data")
		}
	}

	// TODO: maybe worth returning an error if we cannot get the signature, as we don't support
	// anonymous functions and closures as targetted functions via indicators anyway
	sig, _ := GetFuncSignature(funcIdent, info)

	return &FuncInfo{
		Package: pkgPath.Path(),
		Pkg:     pkgPath,
		//Type: nil,
		Name:      funcName,
		Signature: sig,
	}, nil
}

func GetFuncSignature(expr ast.Expr, info *types.Info) (*types.Signature, error) {
	switch expr := expr.(type) {
	case *ast.Ident:
		obj := info.ObjectOf(expr)
		return getSignatureFromObject(obj)
	case *ast.CallExpr:
		if ident, ok := expr.Fun.(*ast.Ident); ok {
			obj := info.ObjectOf(ident)
			return getSignatureFromObject(obj)
		}
	}

	return nil, errors.New("unable to get signature from expression")
}

func getSignatureFromObject(obj types.Object) (*types.Signature, error) {
	switch obj := obj.(type) {
	case *types.Func:
		return obj.Type().(*types.Signature), nil
	case *types.Var:
		if sig, ok := obj.Type().(*types.Signature); ok {
			return sig, nil
		}
	}
	return nil, errors.New("object is not a function or does not have a signature")
}

func GetName(e ast.Expr) string {
	ident, ok := e.(*ast.Ident)
	if !ok {
		return ""
	} else {
		return ident.Name
	}
}

// TODO: Lots of repeated code that we can refactor here
// Further, this is likely not sufficient if used for more general purposes (outside wally) as
// there are parts of some statements (i.e. a ForStmt Post) that are not handled here
func GetExprsFromStmt(stmt ast.Stmt) []*ast.CallExpr {
	var result []*ast.CallExpr
	switch s := stmt.(type) {
	case *ast.ExprStmt:
		ce := callExprFromExpr(s.X)
		if ce != nil {
			result = append(result, ce...)
		}
	case *ast.SwitchStmt:
		for _, iclause := range s.Body.List {
			clause := iclause.(*ast.CaseClause)
			for _, stm := range clause.Body {
				bodyExps := GetExprsFromStmt(stm)
				if len(bodyExps) > 0 {
					result = append(result, bodyExps...)
				}
			}
		}
	case *ast.IfStmt:
		condCe := callExprFromExpr(s.Cond)
		if condCe != nil {
			result = append(result, condCe...)
		}
		if s.Init != nil {
			initCe := GetExprsFromStmt(s.Init)
			if len(initCe) > 0 {
				result = append(result, initCe...)
			}
		}
		if s.Else != nil {
			elseCe := GetExprsFromStmt(s.Else)
			if len(elseCe) > 0 {
				result = append(result, elseCe...)
			}
		}
		ces := GetExprsFromStmt(s.Body)
		if len(ces) > 0 {
			result = append(result, ces...)
		}
	case *ast.BlockStmt:
		for _, stm := range s.List {
			ce := GetExprsFromStmt(stm)
			if ce != nil {
				result = append(result, ce...)
			}
		}
	case *ast.AssignStmt:
		for _, rhs := range s.Rhs {
			ce := callExprFromExpr(rhs)
			if ce != nil {
				result = append(result, ce...)
			}
		}
		for _, lhs := range s.Lhs {
			ce := callExprFromExpr(lhs)
			if ce != nil {
				result = append(result, ce...)
			}
		}
	case *ast.ReturnStmt:
		for _, retResult := range s.Results {
			ce := callExprFromExpr(retResult)
			if ce != nil {
				result = append(result, ce...)
			}
		}
	case *ast.ForStmt:
		ces := GetExprsFromStmt(s.Body)
		if len(ces) > 0 {
			result = append(result, ces...)
		}
	case *ast.RangeStmt:
		ces := GetExprsFromStmt(s.Body)
		if len(ces) > 0 {
			result = append(result, ces...)
		}
	case *ast.SelectStmt:
		for _, clause := range s.Body.List {
			//ces := GetExprsFromStmt(clause)
			if cc, ok := clause.(*ast.CommClause); ok {
				for _, stm := range cc.Body {
					bodyExps := GetExprsFromStmt(stm)
					if len(bodyExps) > 0 {
						result = append(result, bodyExps...)
					}
				}
			}
		}
	case *ast.LabeledStmt:
		ces := GetExprsFromStmt(s.Stmt)
		if len(ces) > 0 {
			result = append(result, ces...)
		}
	}
	return result
}

func callExprFromExpr(e ast.Expr) []*ast.CallExpr {
	switch e := e.(type) {
	case *ast.CallExpr:
		// This loop makes sure we obtain CEs when in the body function literal used
		// as arguments to CEs. See https://github.com/hashicorp/nomad/blob/d34788896f8892377a9039b81a65abd7a913b3cc/nomad/csi_endpoint.go#L1633
		// for an example
		for _, v := range e.Args {
			if rr, ok := v.(*ast.FuncLit); ok {
				return GetExprsFromStmt(rr.Body)
			}
		}
		return append([]*ast.CallExpr{}, e)
	case *ast.FuncLit:
		return GetExprsFromStmt(e.Body)
	}
	return nil
}

func GetFunctionFromCallInstruction(callInstr ssa.CallInstruction) *ssa.Function {
	callCommon := callInstr.Common()
	if callCommon == nil {
		return nil
	}

	return callCommon.StaticCallee()
}

func SiteMatchesFunc(site ssa.CallInstruction, function *ssa.Function) bool {
	callCommon := site.Common()
	if callCommon == nil {
		return false
	}

	siteFunc := GetFunctionFromSite(site)
	return siteFunc != nil && siteFunc == function ||
		callCommon.Method != nil && callCommon.Method.Name() == function.Name()
}

func GetFunctionFromSite(site ssa.CallInstruction) *ssa.Function {
	callCommon := site.Common()
	if callCommon == nil {
		return nil
	}

	if !callCommon.IsInvoke() {
		return callCommon.StaticCallee()
	} else {
		receiverType := callCommon.Method.Type().(*types.Signature).Recv().Type()

		if ptrType, ok := receiverType.(*types.Pointer); ok {
			receiverType = ptrType.Elem()
		}

		// Get the method set of the receiver type
		methodSet := types.NewMethodSet(receiverType)
		for i := 0; i < methodSet.Len(); i++ {
			method := methodSet.At(i)
			if method.Obj().Name() == callCommon.Method.Name() {
				// Ensure method.Obj() is of type *types.Func
				if funcObj, ok := method.Obj().(*types.Func); ok {
					// Use the package's program to find the corresponding ssa.Function
					if fn := site.Parent().Prog.FuncValue(funcObj); fn != nil {
						return fn
					}
				}
			}
		}

		return nil
	}
}

func IsClosure(function *ssa.Function) bool {
	return strings.Contains(function.Name(), "$") && !IsBoundFunc(function)
}

// IsBoundFunc reports whether function is a synthetic wrapper the SSA builder
// generates for a bound method value (e.g. `f := obj.Method`). These wrappers
// have no lexical parent and no *ssa.Package (Func.Package() is nil), so they
// look like closures (their name contains "$") but must not be treated as one:
// walking Parent() on them panics. They also lack a source position, so
// callers must fall back to the call site's position instead of the
// function's own position.
func IsBoundFunc(function *ssa.Function) bool {
	return strings.HasSuffix(function.Name(), "$bound")
}

func getModuleName(pkg *packages.Package) (string, error) {
	if pkg.Module != nil {
		return pkg.Module.Path, nil
	}
	return "", fmt.Errorf("module not found for package %s", pkg.PkgPath)
}

func inStd(node *callgraph.Node) bool {
	pkg, _ := build.Import(node.Func.Pkg.Pkg.Path(), "", 0)
	return pkg.Goroot
}
