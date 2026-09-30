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

// TestFunctionIndex_ResolvesEmbeddedMethodDespiteSyntheticThunkTie is a
// regression test for a second instance of the same unstable-sort-tie bug
// class the $bound exclusion already fixes above, but for a different
// synthetic function kind: a type embedding another's pointer (here,
// embed.Wrapper embedding *embed.Base) gets a synthetic method-promotion
// thunk for each promoted method, positioned at the SAME line as the real
// method since it has no declaration of its own -- sampleapp/embed's
// Handle has three *ssa.Function entries at its single declaration line
// (the real method, plus two promotion thunks with no package of their
// own). Left unindexed-against, sort.Slice's instability made which of
// the three Resolve's binary search landed on a coin flip across runs --
// confirmed against a real, large codebase where this intermittently broke
// a working right-click resolution with no code change in between.
func TestFunctionIndex_ResolvesEmbeddedMethodDespiteSyntheticThunkTie(t *testing.T) {
	nav, _ := buildSampleappNavigator(t)
	idx := live.NewFunctionIndex(nav)

	for i := 0; i < 10; i++ {
		pkg, fn, recv, ok := idx.Resolve("embed/embed.go", 13)
		if !ok {
			t.Fatalf("run %d: expected embed/embed.go:13 to resolve", i)
		}
		if pkg != "github.com/hex0punk/wally/sampleapp/embed" || fn != "Handle" || recv != "Base" {
			t.Fatalf("run %d: got pkg=%q fn=%q recv=%q, want pkg=.../embed fn=Handle recv=Base", i, pkg, fn, recv)
		}
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

	results := idx.Search("crossinterface", 10, nil)
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
	results := idx.Search("handle", 10, nil)
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
	results := idx.Search("run", 1, nil)
	if len(results) != 1 {
		t.Fatalf("got %d results, want exactly 1 (limit)", len(results))
	}
}

func TestFunctionIndex_SearchEmptyQueryReturnsNoResults(t *testing.T) {
	nav, _ := buildSampleappNavigator(t)
	idx := live.NewFunctionIndex(nav)

	if results := idx.Search("", 50, nil); len(results) != 0 {
		t.Fatalf("expected no results for an empty query, got %d", len(results))
	}
}

func TestFunctionIndex_SearchFiltersByPackage(t *testing.T) {
	nav, _ := buildSampleappNavigator(t)
	idx := live.NewFunctionIndex(nav)

	// "run" matches RunBound/RunCrossInterface/RunAll in the root sampleapp
	// package plus RunSafely (safe) and RunWithWorker (caller) -- restricting
	// to just the caller package should leave exactly RunWithWorker.
	results := idx.Search("run", 50, []string{"github.com/hex0punk/wally/sampleapp/caller"})
	if len(results) != 1 {
		t.Fatalf("got %d results restricted to the caller package, want 1: %+v", len(results), results)
	}
	if results[0].Function != "RunWithWorker" {
		t.Fatalf("got Function=%q, want RunWithWorker", results[0].Function)
	}
}

func TestFunctionIndex_SearchEmptyPackageFilterMeansNoFilter(t *testing.T) {
	nav, _ := buildSampleappNavigator(t)
	idx := live.NewFunctionIndex(nav)

	all := idx.Search("run", 50, nil)
	explicit := idx.Search("run", 50, []string{})
	if len(all) != len(explicit) {
		t.Fatalf("nil and empty-slice package filters should behave identically (no filter), got %d vs %d", len(all), len(explicit))
	}
}

func TestFunctionIndex_Packages(t *testing.T) {
	nav, _ := buildSampleappNavigator(t)
	idx := live.NewFunctionIndex(nav)

	pkgs := idx.Packages()
	want := "github.com/hex0punk/wally/sampleapp/caller"
	found := false
	for _, p := range pkgs {
		if p == want {
			found = true
		}
	}
	if !found {
		t.Fatalf("expected %q among Packages(), got %+v", want, pkgs)
	}
	for i := 1; i < len(pkgs); i++ {
		if pkgs[i-1] > pkgs[i] {
			t.Fatalf("Packages() not sorted: %q came before %q", pkgs[i-1], pkgs[i])
		}
	}
}

func TestFunctionIndex_SearchExcludesClosuresAndBoundWrappers(t *testing.T) {
	nav, _ := buildSampleappNavigator(t)
	idx := live.NewFunctionIndex(nav)

	// Neither a closure nor a $bound wrapper is independently queryable via
	// --pkg/--func, so neither should ever surface as a search result --
	// every real *ssa.Function's own Name() is "$"-free; only synthetic
	// closures ("...$1") and bound wrappers ("Handle$bound") carry one.
	for _, r := range idx.Search("handle", 200, nil) {
		if strings.Contains(r.Function, "$") {
			t.Fatalf("expected no synthetic closure/bound entries in search results, got %+v", r)
		}
	}
}
