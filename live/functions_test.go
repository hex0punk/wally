package live_test

import (
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
