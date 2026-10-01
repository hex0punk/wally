package live_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/hex0punk/wally/live"
	"github.com/hex0punk/wally/navigator"
)

// sharedNav/sharedSampleappDir back buildSampleappNavigator -- see TestMain.
var (
	sharedNav          *navigator.Navigator
	sharedSampleappDir string
)

// TestMain builds the real sampleapp Navigator exactly ONCE for this
// package's whole test binary, rather than once per test. Building it per
// test (each a fresh Navigator.Build, i.e. a fresh go/packages.Load +
// SSA construction) intermittently produced an incomplete result -- a
// method reached only via a bound method value (bound.Handler.Handle, see
// wallylib.IsBoundFunc) occasionally had no SSA body indexed, at roughly a
// 1-in-8 rate, regardless of which test hit it or whether other tests ran
// alongside it -- reproducing only on a second-or-later real Build() call
// within one process, never as a first/only call, and never as a
// standalone repro outside `go test`. That smells like a rare
// upstream/environmental nondeterminism in repeated Build() invocations
// (arguably worth its own investigation some day -- wally shell's own
// `reload` also calls Build() again within one process -- but out of
// scope for what this test suite needs to prove). Building once, like a
// real wally process's own single-build-many-queries lifecycle, sidesteps
// it entirely rather than papering over a result that can't be trusted.
//
// sampleapp has its own go.mod (a separate module from wally's own), so
// go/packages can't resolve a cross-module relative pattern like
// "../sampleapp/..." from this package's directory -- every real
// invocation of wally against it this session cd'd into sampleapp/ first
// (e.g. "cd sampleapp && ../wally shell -p ./..."), and this test does the
// same for the whole run, restoring the original directory once all tests
// finish.
func TestMain(m *testing.M) {
	orig, err := os.Getwd()
	if err != nil {
		panic(err)
	}
	sharedSampleappDir, err = filepath.Abs("../sampleapp")
	if err != nil {
		panic(err)
	}
	if err := os.Chdir(sharedSampleappDir); err != nil {
		panic(err)
	}

	sharedNav = navigator.NewNavigator(0, nil)
	sharedNav.RunSSA = true
	sharedNav.CallgraphAlg = "cha"
	sharedNav.Build([]string{"./..."})

	code := m.Run()

	if err := os.Chdir(orig); err != nil {
		panic(err)
	}
	os.Exit(code)
}

// buildSampleappNavigator returns the shared sampleapp Navigator built
// once in TestMain, plus its absolute directory (cwd for the whole test
// run -- see TestMain). Kept as a function, not a bare variable access, so
// every existing call site reads the same either way.
func buildSampleappNavigator(t *testing.T) (nav *navigator.Navigator, sampleappDir string) {
	t.Helper()
	return sharedNav, sharedSampleappDir
}

func TestSourceIndex_ResolvesAnalyzedFile(t *testing.T) {
	nav, sampleappDir := buildSampleappNavigator(t)
	idx := live.NewSourceIndex(nav)

	absMain := filepath.Join(sampleappDir, "main.go")

	// Absolute form (how a target position built from token.Position.String()
	// can appear) resolves directly.
	if _, ok := idx.Resolve(absMain); !ok {
		t.Fatalf("expected absolute path %q to resolve", absMain)
	}

	// Relative-to-cwd form (how GetFormattedPos/GetFormattedPosFromFunc
	// format every call-path frame) must also resolve to the same file --
	// this is just "main.go" once cwd is sampleapp/ itself.
	resolved, ok := idx.Resolve("main.go")
	if !ok {
		t.Fatal(`expected relative path "main.go" to resolve`)
	}
	if resolved != filepath.Clean(absMain) {
		t.Fatalf("resolved to %q, want %q", resolved, absMain)
	}
}

func TestSourceIndex_RejectsUnanalyzedFile(t *testing.T) {
	nav, _ := buildSampleappNavigator(t)
	idx := live.NewSourceIndex(nav)

	// A real, existing Go file on disk -- just not one wally's sampleapp-only
	// build ever parsed. The allowlist must reject it purely because it's
	// not a member, not because it fails an existence check.
	absOther, err := filepath.Abs("../reporter/pretty.go")
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := idx.Resolve(absOther); ok {
		t.Fatalf("expected %q (real file, never analyzed) to be rejected", absOther)
	}
}

func TestSourceIndex_RejectsPathTraversal(t *testing.T) {
	nav, _ := buildSampleappNavigator(t)
	idx := live.NewSourceIndex(nav)

	// A traversal payload that Cleans() down to a real, sensitive file
	// outside anything wally analyzed. This must be rejected by allowlist
	// membership, not by pattern-blocking "..".
	traversal := filepath.Join("..", "..", "..", "..", "..", "..", "etc", "passwd")
	if _, ok := idx.Resolve(traversal); ok {
		t.Fatalf("expected traversal path %q to be rejected", traversal)
	}
}

func TestSourceIndex_EmptyFileRejected(t *testing.T) {
	nav, _ := buildSampleappNavigator(t)
	idx := live.NewSourceIndex(nav)

	if _, ok := idx.Resolve(""); ok {
		t.Fatal("expected empty file to be rejected")
	}
}
