package live_test

import (
	"testing"

	"github.com/hex0punk/wally/live"
	"github.com/hex0punk/wally/navigator"
)

// TestFilterPathsThroughFunction exercises the real sampleapp path
// main.main -> main.RunCrossInterface -> caller.RunWithWorker ->
// target.(*Client).DoWork -- caller.RunWithWorker dispatches to DoWork
// through its own, independently-declared interface (see caller.Worker's
// doc comment), the exact shape a source→sink check needs to see through.
func TestFilterPathsThroughFunction(t *testing.T) {
	nav, _ := buildSampleappNavigator(t)

	q := navigator.DefaultQueryParams()
	q.Pkg = "github.com/hex0punk/wally/sampleapp/target"
	q.Func = "DoWork"
	q.RecvType = "Client"

	matches := nav.Query(q)
	if len(matches) == 0 {
		t.Fatal("expected at least one match for target.(*Client).DoWork")
	}

	t.Run("keeps paths through a real intermediate hop", func(t *testing.T) {
		through := live.FilterPathsThroughFunction(matches, "github.com/hex0punk/wally/sampleapp/caller", "RunWithWorker", "")
		if len(through) == 0 {
			t.Fatal("expected at least one match to survive filtering through caller.RunWithWorker")
		}
		for _, m := range through {
			if m.SSA == nil || m.SSA.CallPaths == nil || len(m.SSA.CallPaths.Paths) == 0 {
				t.Fatalf("expected a surviving match to keep at least one path, got %+v", m)
			}
		}
	})

	t.Run("keeps paths through the outer entry point too", func(t *testing.T) {
		through := live.FilterPathsThroughFunction(matches, "github.com/hex0punk/wally/sampleapp", "RunCrossInterface", "")
		if len(through) == 0 {
			t.Fatal("expected at least one match to survive filtering through main.RunCrossInterface")
		}
	})

	t.Run("drops every match when the source is never on any path", func(t *testing.T) {
		none := live.FilterPathsThroughFunction(matches, "github.com/hex0punk/wally/sampleapp/safe", "RunSafely", "")
		if len(none) != 0 {
			t.Fatalf("expected zero matches for a source never on any path, got %d: %+v", len(none), none)
		}
	})

	t.Run("a wrong receiver type on an otherwise-matching name also drops it", func(t *testing.T) {
		// caller.RunWithWorker is a bare function (no receiver) -- asking
		// for it with a receiver type must not match.
		none := live.FilterPathsThroughFunction(matches, "github.com/hex0punk/wally/sampleapp/caller", "RunWithWorker", "SomeType")
		if len(none) != 0 {
			t.Fatalf("expected zero matches for a receiver-type mismatch, got %d: %+v", len(none), none)
		}
	})
}
