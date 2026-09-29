package live_test

import (
	"strings"
	"testing"

	"github.com/hex0punk/wally/live"
)

func TestFunctionIndex_ResolvesLineInsideFunctionBody(t *testing.T) {
	nav, _ := buildSampleappNavigator(t)
	idx := live.NewFunctionIndex(nav)

	// Line 46 is inside RunCrossInterface's body (caller.RunWithWorker(c, 1)),
	// not its line-44 declaration -- proves this resolves by enclosing range,
	// not by an exact declaration-line match.
	pkg, fn, recv, ok := idx.Resolve("main.go", 46)
	if !ok {
		t.Fatal("expected line 46 to resolve")
	}
	if pkg != "github.com/hex0punk/wally/sampleapp" || fn != "RunCrossInterface" || recv != "" {
		t.Fatalf("got pkg=%q fn=%q recv=%q, want pkg=github.com/hex0punk/wally/sampleapp fn=RunCrossInterface recv=\"\"", pkg, fn, recv)
	}
}

func TestFunctionIndex_ResolvesMethodWithReceiverType(t *testing.T) {
	nav, _ := buildSampleappNavigator(t)
	idx := live.NewFunctionIndex(nav)

	// bound/bound.go:8 is inside (*Handler).Handle's body.
	pkg, fn, recv, ok := idx.Resolve("bound/bound.go", 8)
	if !ok {
		t.Fatal("expected bound/bound.go:8 to resolve")
	}
	if pkg != "github.com/hex0punk/wally/sampleapp/bound" || fn != "Handle" || recv != "Handler" {
		t.Fatalf("got pkg=%q fn=%q recv=%q, want pkg=.../bound fn=Handle recv=Handler", pkg, fn, recv)
	}
}

func TestFunctionIndex_ClosureWalksUpToEnclosingFunction(t *testing.T) {
	nav, _ := buildSampleappNavigator(t)
	idx := live.NewFunctionIndex(nav)

	// main.go:62 is inside the anonymous func literal passed to
	// safe.RunSafely from within printCharSafe -- the closure itself isn't
	// independently queryable, so this must resolve to printCharSafe.
	pkg, fn, recv, ok := idx.Resolve("main.go", 62)
	if !ok {
		t.Fatal("expected line 62 (inside the closure) to resolve")
	}
	if pkg != "github.com/hex0punk/wally/sampleapp" || fn != "printCharSafe" || recv != "" {
		t.Fatalf("got pkg=%q fn=%q recv=%q, want pkg=github.com/hex0punk/wally/sampleapp fn=printCharSafe recv=\"\"", pkg, fn, recv)
	}
}

func TestFunctionIndex_NoFunctionBeforeFirstDeclaration(t *testing.T) {
	nav, _ := buildSampleappNavigator(t)
	idx := live.NewFunctionIndex(nav)

	// Line 2 is a blank line before any function declaration in the file.
	if _, _, _, ok := idx.Resolve("main.go", 2); ok {
		t.Fatal("expected line 2 (before any function) to not resolve")
	}
}

func TestFunctionIndex_UnknownFile(t *testing.T) {
	nav, _ := buildSampleappNavigator(t)
	idx := live.NewFunctionIndex(nav)

	if _, _, _, ok := idx.Resolve("does/not/exist.go", 1); ok {
		t.Fatal("expected an unindexed file to not resolve")
	}
}

func TestFunctionIndex_SearchMatchesCaseInsensitiveSubstring(t *testing.T) {
	nav, _ := buildSampleappNavigator(t)
	idx := live.NewFunctionIndex(nav)

	results := idx.Search("crossinterface", 10)
	if len(results) != 1 {
		t.Fatalf("got %d results, want 1: %+v", len(results), results)
	}
	if results[0].Function != "RunCrossInterface" {
		t.Fatalf("got Function=%q, want RunCrossInterface", results[0].Function)
	}
}

func TestFunctionIndex_SearchMatchesMethodByReceiverDotName(t *testing.T) {
	nav, _ := buildSampleappNavigator(t)
	idx := live.NewFunctionIndex(nav)

	// "handle" is only a substring of "Handler.Handle" once RecvType is
	// folded into the search haystack -- proves method results are
	// searchable by more than just the bare method name.
	results := idx.Search("handle", 10)
	found := false
	for _, r := range results {
		if r.Function == "Handle" && r.RecvType == "Handler" {
			found = true
			if r.Pkg != "github.com/hex0punk/wally/sampleapp/bound" {
				t.Fatalf("got Pkg=%q, want .../bound", r.Pkg)
			}
			if r.File == "" || r.Line == 0 {
				t.Fatalf("expected a real file/line, got File=%q Line=%d", r.File, r.Line)
			}
		}
	}
	if !found {
		t.Fatalf("expected (*Handler).Handle among results, got %+v", results)
	}
}

func TestFunctionIndex_SearchCapsAtLimit(t *testing.T) {
	nav, _ := buildSampleappNavigator(t)
	idx := live.NewFunctionIndex(nav)

	// "run" matches at least RunBound, RunCrossInterface, RunAll.
	results := idx.Search("run", 1)
	if len(results) != 1 {
		t.Fatalf("got %d results, want exactly 1 (limit)", len(results))
	}
}

func TestFunctionIndex_SearchEmptyQueryReturnsNoResults(t *testing.T) {
	nav, _ := buildSampleappNavigator(t)
	idx := live.NewFunctionIndex(nav)

	if results := idx.Search("", 50); len(results) != 0 {
		t.Fatalf("expected no results for an empty query, got %d", len(results))
	}
}

func TestFunctionIndex_SearchExcludesClosuresAndBoundWrappers(t *testing.T) {
	nav, _ := buildSampleappNavigator(t)
	idx := live.NewFunctionIndex(nav)

	// Neither a closure nor a $bound wrapper is independently queryable via
	// --pkg/--func, so neither should ever surface as a search result --
	// every real *ssa.Function's own Name() is "$"-free; only synthetic
	// closures ("...$1") and bound wrappers ("Handle$bound") carry one.
	for _, r := range idx.Search("handle", 200) {
		if strings.Contains(r.Function, "$") {
			t.Fatalf("expected no synthetic closure/bound entries in search results, got %+v", r)
		}
	}
}
