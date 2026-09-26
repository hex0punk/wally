package reporter

import (
	"fmt"
	"os"
	"strings"

	"github.com/hex0punk/wally/match"
	"github.com/pterm/pterm"
	"golang.org/x/term"
)

func init() {
	// pterm/gookit's own tty detection doesn't cover every shell this runs
	// in reliably, so decide explicitly: no point emitting escape codes into
	// a pipe, a log file, or `wally ... | less` without -R.
	if !term.IsTerminal(int(os.Stdout.Fd())) {
		pterm.DisableColor()
	}
}

// neutralStyle replaces a widget's own TextStyle so it doesn't re-wrap text
// that already carries its own inline color codes -- see the tree/box calls
// below, which bake per-line color into the content itself for per-node
// control pterm's uniform widget-level styles can't express.
var neutralStyle = pterm.NewStyle()

func PrintResults(matches []match.RouteMatch) {
	for _, m := range matches {
		PrintMach(m)
	}
	count := fmt.Sprintf("Total Results: %d", len(matches))
	if len(matches) > 0 {
		count = pterm.NewStyle(pterm.FgGreen, pterm.Bold).Sprint(count)
	} else {
		count = pterm.FgGray.Sprint(count)
	}
	fmt.Println(count)
}

func PrintMach(m match.RouteMatch) {
	title := fmt.Sprintf("%s.%s", m.Indicator.Package, m.Indicator.Function)

	var body strings.Builder
	writeField(&body, "Package", m.Indicator.Package)
	writeField(&body, "Function", m.Indicator.Function)
	writeField(&body, "Module", m.Module)

	if len(m.Params) == 0 {
		writeField(&body, "Params", pterm.FgGray.Sprint("<none>"))
	} else {
		writeField(&body, "Params", "")
		for k, v := range m.Params {
			if v == "" {
				v = pterm.FgGray.Sprint("<could not resolve>")
			}
			if k == "" {
				k = "<not specified>"
			}
			fmt.Fprintf(&body, "  %s %s: %s\n", pterm.FgGray.Sprint("-"), k, v)
		}
	}

	// TODO: This is printing the values from the indicator. That's fine,
	// and it works, but it should print values from those captured during
	// navigator, just in case.
	enclosedBy := m.EnclosedBy
	if m.SSA != nil && m.SSA.EnclosedByFunc != nil {
		enclosedBy = m.SSA.EnclosedByFunc.String()
	}
	writeField(&body, "Enclosed by", enclosedBy)
	writeField(&body, "Position", fmt.Sprintf("%s:%d", m.Pos.Filename, m.Pos.Line))

	pterm.DefaultBox.
		WithTitle(pterm.Bold.Sprint(title)).
		WithTitleTopCenter().
		WithBoxStyle(pterm.NewStyle(pterm.FgCyan)).
		WithTextStyle(neutralStyle).
		Println(strings.TrimSuffix(body.String(), "\n"))

	if m.SSA != nil && m.SSA.CallPaths != nil && len(m.SSA.CallPaths.Paths) > 0 {
		printCallPaths(m)
	}

	fmt.Println()
}

func writeField(b *strings.Builder, label, value string) {
	fmt.Fprintf(b, "%s %s\n", pterm.Bold.Sprint(label+":"), value)
}

func printCallPaths(m match.RouteMatch) {
	paths := m.SSA.CallPaths.Paths

	countLabel := fmt.Sprintf("Possible Paths: %d", len(paths))
	if m.SSA.PathLimited {
		countLabel = fmt.Sprintf("Possible Paths (path limited): %d", len(paths))
	}
	fmt.Println(pterm.Bold.Sprint(countLabel))

	for i, p := range paths {
		fmt.Println(pathHeader(i+1, p))
		root := buildPathTree(p, m.SSA.TargetPos)
		tree := pterm.DefaultTree.WithRoot(root).WithTextStyle(neutralStyle)
		_ = tree.Render()
	}
}

// pathHeader renders the same badges the plain-text reporter always has --
// RECOVERABLE, node/filter limited, and the two import-verification
// warnings -- colored by how much trust each one implies.
func pathHeader(index int, p *match.CallPath) string {
	var b strings.Builder
	fmt.Fprintf(&b, "  Path %d", index)

	if p.NodeLimited {
		b.WriteString(pterm.FgGray.Sprint(" (node limited)"))
	}
	if p.FilterLimited {
		b.WriteString(pterm.FgGray.Sprint(" (filter limited)"))
	}
	if p.Recoverable {
		b.WriteString(pterm.FgGreen.Sprint(" (RECOVERABLE)"))
	}

	unconfirmedCount := len(p.Nodes) - p.ConfirmedDepth
	switch {
	case p.VerificationAttempted && p.ConfirmedDepth == 0 && len(p.Nodes) > 0:
		warning := " (!! NO IMPORT PATH FOUND for even the call into the target -- likely a cha/vta false positive from a widely-implemented interface, verify against source before trusting this !!)"
		b.WriteString(pterm.NewStyle(pterm.FgRed, pterm.Bold).Sprint(warning))
	case p.VerificationAttempted && unconfirmedCount > 0:
		warning := fmt.Sprintf(" (%d outer frame(s) beyond the [unconfirmed] marker below have no direct import to what they supposedly call -- likely generic/shared framework code a cha/vta over-approximation attached, not necessarily fake, but not confirmed either)", unconfirmedCount)
		b.WriteString(pterm.FgYellow.Sprint(warning))
	}

	b.WriteString(":")
	return b.String()
}

// buildPathTree builds the nested pterm.TreeNode chain for one call path,
// innermost (the call into the matched target) first, growing outward to
// the outermost caller, which ends up as the returned root -- matching how
// the plain-text reporter always walked paths.Nodes from its last index
// down to 0.
func buildPathTree(p *match.CallPath, targetPos string) pterm.TreeNode {
	node := pterm.TreeNode{Text: pterm.NewStyle(pterm.FgCyan, pterm.Bold).Sprint(targetPos)}

	for x := 0; x < len(p.Nodes); x++ {
		unconfirmed := p.VerificationAttempted && x >= p.ConfirmedDepth
		node = pterm.TreeNode{
			Text:     styleNodeText(p.Nodes[x].NodeString, unconfirmed),
			Children: []pterm.TreeNode{node},
		}
	}

	return node
}

func styleNodeText(nodeString string, unconfirmed bool) string {
	nodeString = strings.ReplaceAll(nodeString, "(recoverable)", pterm.FgGreen.Sprint("(recoverable)"))
	if unconfirmed {
		return pterm.FgYellow.Sprint("[unconfirmed] ") + nodeString
	}
	return nodeString
}
