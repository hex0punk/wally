package live_test

import (
	"testing"

	"github.com/hex0punk/wally/live"
	"github.com/hex0punk/wally/match"
	"github.com/hex0punk/wally/wallynode"
)

func node(nodeString string) wallynode.WallyNode {
	return wallynode.WallyNode{NodeString: nodeString}
}

func matchWithPaths(targetPos string, paths ...*match.CallPath) match.RouteMatch {
	return match.RouteMatch{
		MatchId: "m-" + targetPos,
		SSA: &match.SSAContext{
			TargetPos: targetPos,
			CallPaths: &match.CallPaths{Paths: paths},
		},
	}
}

func findNode(t *testing.T, els live.Elements, label string) live.NodeData {
	t.Helper()
	for _, n := range els.Nodes {
		if n.Data.Label == label {
			return n.Data
		}
	}
	t.Fatalf("no node with label %q in %+v", label, els.Nodes)
	return live.NodeData{}
}

func labelByID(els live.Elements, id string) string {
	for _, n := range els.Nodes {
		if n.Data.ID == id {
			return n.Data.Label
		}
	}
	return ""
}

func findEdge(t *testing.T, els live.Elements, sourceLabel, targetLabel string) live.EdgeData {
	t.Helper()
	for _, e := range els.Edges {
		if labelByID(els, e.Data.Source) == sourceLabel && labelByID(els, e.Data.Target) == targetLabel {
			return e.Data
		}
	}
	t.Fatalf("no edge %s -> %s in %+v", sourceLabel, targetLabel, els.Edges)
	return live.EdgeData{}
}

func TestBuildGraph_LinearPath(t *testing.T) {
	// Nodes is innermost-first: "A" calls the target, "B" calls "A".
	p := &match.CallPath{Nodes: []wallynode.WallyNode{node("A"), node("B")}}
	m := matchWithPaths("T", p)

	els, paths, matches := live.BuildGraph([]match.RouteMatch{m})

	if len(matches) != 1 {
		t.Fatalf("want 1 match, got %d", len(matches))
	}
	if len(paths) != 1 {
		t.Fatalf("want 1 path, got %d", len(paths))
	}
	// Display order is outermost -> target, matching the CLI tree's
	// top-to-bottom read order.
	want := []string{"B", "A", "T"}
	if got := paths[0].Nodes; !equal(got, want) {
		t.Fatalf("path nodes = %v, want %v", got, want)
	}
	if len(els.Nodes) != 3 {
		t.Fatalf("want 3 graph nodes, got %d: %+v", len(els.Nodes), els.Nodes)
	}
	if len(els.Edges) != 2 {
		t.Fatalf("want 2 edges, got %d: %+v", len(els.Edges), els.Edges)
	}
	findEdge(t, els, "A", "T") // the call into the target
	findEdge(t, els, "B", "A")
}

func TestBuildGraph_MergesSharedOuterFrame_ConfirmedAnywhereWins(t *testing.T) {
	// Path 1: B is beyond the confirmed prefix (unconfirmed).
	p1 := &match.CallPath{
		Nodes:                 []wallynode.WallyNode{node("A1"), node("B")},
		ConfirmedDepth:        1,
		VerificationAttempted: true,
	}
	// Path 2: B is within the confirmed prefix (confirmed).
	p2 := &match.CallPath{
		Nodes:                 []wallynode.WallyNode{node("A2"), node("B")},
		ConfirmedDepth:        2,
		VerificationAttempted: true,
	}
	m := matchWithPaths("T", p1, p2)

	els, _, _ := live.BuildGraph([]match.RouteMatch{m})

	// One shared "B" node and one shared "T" node, plus A1 and A2 -- 4 total.
	if len(els.Nodes) != 4 {
		t.Fatalf("want 4 merged nodes, got %d: %+v", len(els.Nodes), els.Nodes)
	}
	b := findNode(t, els, "B")
	if b.Unconfirmed {
		t.Fatalf("B should be confirmed (confirmed-anywhere-wins via path 2), got Unconfirmed=true")
	}
	if len(b.Paths) != 2 {
		t.Fatalf("B should belong to both paths, got %v", b.Paths)
	}
}

func TestBuildGraph_UnconfirmedPrefix(t *testing.T) {
	p := &match.CallPath{
		Nodes:                 []wallynode.WallyNode{node("A"), node("B"), node("C")},
		ConfirmedDepth:        1,
		VerificationAttempted: true,
	}
	m := matchWithPaths("T", p)

	els, paths, _ := live.BuildGraph([]match.RouteMatch{m})

	if findNode(t, els, "A").Unconfirmed {
		t.Fatal("A (within confirmed prefix) should not be unconfirmed")
	}
	if !findNode(t, els, "B").Unconfirmed {
		t.Fatal("B (beyond confirmed prefix) should be unconfirmed")
	}
	if !findNode(t, els, "C").Unconfirmed {
		t.Fatal("C (beyond confirmed prefix) should be unconfirmed")
	}
	if paths[0].UnconfirmedCount != 2 {
		t.Fatalf("UnconfirmedCount = %d, want 2", paths[0].UnconfirmedCount)
	}
	if paths[0].NoImportPathFound {
		t.Fatal("NoImportPathFound should be false when ConfirmedDepth > 0")
	}
}

func TestBuildGraph_NoImportPathFound(t *testing.T) {
	p := &match.CallPath{
		Nodes:                 []wallynode.WallyNode{node("A")},
		ConfirmedDepth:        0,
		VerificationAttempted: true,
	}
	m := matchWithPaths("T", p)

	els, paths, _ := live.BuildGraph([]match.RouteMatch{m})

	if !paths[0].NoImportPathFound {
		t.Fatal("NoImportPathFound should be true when ConfirmedDepth == 0")
	}
	edge := findEdge(t, els, "A", "T")
	if !edge.NoImport {
		t.Fatal("the edge into the target should be flagged NoImport")
	}
}

func TestBuildGraph_RecoverableMarker(t *testing.T) {
	p := &match.CallPath{
		Nodes: []wallynode.WallyNode{node("main.[f] (recoverable) main.go:1:1")},
	}
	m := matchWithPaths("main.[Target] (recoverable) main.go:2:2", p)

	els, _, _ := live.BuildGraph([]match.RouteMatch{m})

	if !findNode(t, els, "main.[f] (recoverable) main.go:1:1").Recoverable {
		t.Fatal("frame with (recoverable) marker should be flagged recoverable")
	}
	if !findNode(t, els, "main.[Target] (recoverable) main.go:2:2").Recoverable {
		t.Fatal("target leaf with (recoverable) marker should be flagged recoverable")
	}
}

func TestBuildGraph_SimpleModeEmptyNodes(t *testing.T) {
	p := &match.CallPath{Nodes: nil}
	m := matchWithPaths("T", p)

	els, paths, _ := live.BuildGraph([]match.RouteMatch{m})

	if len(els.Nodes) != 1 || els.Nodes[0].Data.Label != "T" {
		t.Fatalf("want exactly the target leaf, got %+v", els.Nodes)
	}
	if len(els.Edges) != 0 {
		t.Fatalf("want no edges, got %+v", els.Edges)
	}
	if got := paths[0].Nodes; !equal(got, []string{"T"}) {
		t.Fatalf("path nodes = %v, want [T]", got)
	}
}

func TestBuildGraph_ParsesFileLineColFromLabels(t *testing.T) {
	// Relative form (GetFormattedPos/GetFormattedPosFromFunc): file:line:col.
	p := &match.CallPath{Nodes: []wallynode.WallyNode{node("pkg.[Frame] pkg/frame.go:12:3")}}
	// Absolute form (token.Position.String(), used for some target
	// positions): file:line, no column.
	m := matchWithPaths("pkg.[Target] /abs/path/target.go:7", p)

	els, _, _ := live.BuildGraph([]match.RouteMatch{m})

	frame := findNode(t, els, "pkg.[Frame] pkg/frame.go:12:3")
	if frame.File != "pkg/frame.go" || frame.Line != 12 || frame.Col != 3 {
		t.Fatalf("frame position = %+v, want file=pkg/frame.go line=12 col=3", frame)
	}

	target := findNode(t, els, "pkg.[Target] /abs/path/target.go:7")
	if target.File != "/abs/path/target.go" || target.Line != 7 || target.Col != 0 {
		t.Fatalf("target position = %+v, want file=/abs/path/target.go line=7 col=0", target)
	}
}

func TestBuildGraph_UnparseablePositionLeavesFileEmpty(t *testing.T) {
	// A raw SSA value's default String() form has no "] " to anchor on.
	p := &match.CallPath{Nodes: []wallynode.WallyNode{node("n86294:(*pkg.Type).Method$bound")}}
	m := matchWithPaths("T", p)

	els, _, _ := live.BuildGraph([]match.RouteMatch{m})

	n := findNode(t, els, "n86294:(*pkg.Type).Method$bound")
	if n.File != "" || n.Line != 0 || n.Col != 0 {
		t.Fatalf("unparseable label should leave File/Line/Col zero, got %+v", n)
	}
}

func TestBuildGraph_NilSSADoesNotPanic(t *testing.T) {
	matches := []match.RouteMatch{
		{MatchId: "no-ssa"},
		{MatchId: "no-callpaths", SSA: &match.SSAContext{TargetPos: "T"}},
	}

	els, paths, matchInfos := live.BuildGraph(matches)

	if len(matchInfos) != 2 {
		t.Fatalf("want 2 match infos even with no graph contribution, got %d", len(matchInfos))
	}
	if len(els.Nodes) != 0 || len(els.Edges) != 0 || len(paths) != 0 {
		t.Fatalf("want an empty graph, got nodes=%v edges=%v paths=%v", els.Nodes, els.Edges, paths)
	}
}

func equal(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}
