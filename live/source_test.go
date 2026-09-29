package live_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/hex0punk/wally/live"
	"github.com/hex0punk/wally/navigator"
)

// buildSampleappNavigator builds a real Navigator against sampleapp --
// SourceIndex is a security boundary, so it's tested against a real
// SSA-built FileSet rather than a hand-constructed fake, to catch any
// integration mistake a fake would paper over (wrong field name, wrong
// FileSet, etc).
//
// sampleapp has its own go.mod (a separate module from wally's own), so
// go/packages can't resolve a cross-module relative pattern like
// "../sampleapp/..." from this package's directory -- every real
// invocation of wally against it this session cd'd into sampleapp/ first
// (e.g. "cd sampleapp && ../wally shell -p ./..."), and this test does the
// same, restoring the original directory when done. It returns the
// absolute sampleapp directory too, since callers need it to build
// absolute/relative test paths that remain valid after the directory is
// restored.
func buildSampleappNavigator(t *testing.T) (nav *navigator.Navigator, sampleappDir string) {
	t.Helper()

	orig, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	sampleappDir, err = filepath.Abs("../sampleapp")
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Chdir(sampleappDir); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := os.Chdir(orig); err != nil {
			t.Fatal(err)
		}
	})

	nav = navigator.NewNavigator(0, nil)
	nav.RunSSA = true
	nav.CallgraphAlg = "cha"
	nav.Build([]string{"./..."})
	return nav, sampleappDir
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
