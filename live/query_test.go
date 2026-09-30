package live_test

import (
	"strings"
	"testing"

	"github.com/hex0punk/wally/live"
	"github.com/hex0punk/wally/navigator"
)

// TestQuery_CallSiteWithNoEnclosingFunctionDoesNotPanic is a regression
// test for a nil-pointer crash in Navigator.Run's module-name resolution.
// The fallback branch dereferenced funcInfo.EnclosedBy unconditionally,
// which panics whenever BOTH of these hold for a matched call site:
//   - the matched function's own package has no module info (true for
//     every standard-library package -- see GetModuleName's own doc
//     comment), so the primary GetModuleName lookup returns "" and the
//     fallback branch runs at all, and
//   - the call site itself has no enclosing function (sampleapp's
//     packageLevelCall = strconv.Itoa(42), a package-level var initializer
//     evaluated before main() ever runs, not inside any func body).
//
// Confirmed against a real, large multi-module codebase where a query for
// an unrelated function happened to match this same shape and permanently
// wedged a live server's query mutex (a panic mid-Query never reached the
// deferred Unlock -- see live/server.go's queryLocked) -- this test exists
// so that class of AST shape can never silently reappear.
func TestQuery_CallSiteWithNoEnclosingFunctionDoesNotPanic(t *testing.T) {
	nav, _ := buildSampleappNavigator(t)

	q := navigator.DefaultQueryParams()
	q.Pkg = "strconv"
	q.Func = "Itoa"

	matches := nav.Query(q) // must not panic
	if len(matches) == 0 {
		t.Fatal("expected at least one match for strconv.Itoa's package-level call site")
	}
}

// TestQuery_BoundFuncNodeHasRealSourcePosition is a regression test for a
// $bound wrapper node coming back with no parseable position at all --
// callmapper.buildWallyNode routed it into the same raw-label fallback as
// any other function with no package of its own (a $bound wrapper's own
// Func.Package() is nil), producing an uninformative label like
// "n123:(*T).Method$bound" with nothing live/graph.go's parsePosition could
// extract a file/line from. The live UI's code viewer could then only ever
// report "No source position available" for it -- confirmed against a real
// codebase where exactly this happened for a $bound node on an otherwise
// perfectly normal query.
//
// sampleapp's own bound.Shared is reachable only through
// (*bound.Handler).Handle, itself only ever invoked as a bound method value
// (see main.RunBound) -- the same shape that broke.
func TestQuery_BoundFuncNodeHasRealSourcePosition(t *testing.T) {
	nav, _ := buildSampleappNavigator(t)

	q := navigator.DefaultQueryParams()
	q.Pkg = "github.com/hex0punk/wally/sampleapp/bound"
	q.Func = "Shared"

	matches := nav.Query(q)
	if len(matches) == 0 {
		t.Fatal("expected at least one match for bound.Shared")
	}

	els, _, _ := live.BuildGraph(matches)

	var boundNode *live.NodeElement
	for i := range els.Nodes {
		if strings.Contains(els.Nodes[i].Data.Label, "$bound") {
			boundNode = &els.Nodes[i]
			break
		}
	}
	if boundNode == nil {
		t.Fatalf("expected a $bound node among the results, got %+v", els.Nodes)
	}
	if boundNode.Data.File == "" || boundNode.Data.Line == 0 {
		t.Fatalf("expected the $bound node to carry a real file/line, got label=%q file=%q line=%d",
			boundNode.Data.Label, boundNode.Data.File, boundNode.Data.Line)
	}
}
