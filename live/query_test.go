package live_test

import (
	"testing"

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
