package cmd

import (
	"fmt"
	"log"

	"github.com/hex0punk/wally/live"
	"github.com/hex0punk/wally/navigator"
	"github.com/spf13/cobra"
)

// Session-level flags for `wally live` -- the same shape as wally shell's
// (see cmd/shell.go), minus --config/--skip-default: every query already
// overwrites RouteIndicators wholesale before FindMatches runs (see
// navigator.Navigator.Query), so a preloaded config has no observable
// effect through this path today.
var (
	livePaths        []string
	liveCallgraphAlg string
	liveExcludePkgs  []string
	liveExcludePos   []string
	liveNoAutoDeps   bool
	liveHost         string
	livePort         int
)

var liveCmd = &cobra.Command{
	Use:   "live",
	Short: "Build the SSA callgraph once and serve a live web UI to query it",
	Long: `Like wally shell, this builds the SSA-based callgraph once and keeps it
resident in memory. Instead of a stdin prompt, it serves an HTTP API and a
small web UI (Cytoscape.js) at the given address: type a package and
function, see the matching call paths rendered as a graph.`,
	Args: func(cmd *cobra.Command, args []string) error {
		return validateCallgraphAlg(liveCallgraphAlg)
	},
	Run: runLive,
}

func init() {
	rootCmd.AddCommand(liveCmd)

	liveCmd.PersistentFlags().StringSliceVarP(&livePaths, "paths", "p", livePaths, "The comma separated package paths to target. Use ./... for current directory and subdirectories")
	liveCmd.PersistentFlags().StringVar(&liveCallgraphAlg, "callgraph-alg", "cha", "cha || rta || vta || static")
	liveCmd.PersistentFlags().StringSliceVar(&liveExcludePkgs, "exclude-pkg", []string{}, "Comma separated list of packages to exclude")
	liveCmd.PersistentFlags().StringSliceVar(&liveExcludePos, "exclude-pos", []string{}, "Comma separated list of position suffixes used for filtering the selected function call matches")
	liveCmd.PersistentFlags().BoolVar(&liveNoAutoDeps, "no-auto-deps", false, "Disable automatic expansion of the SSA build set to the same-module transitive closure of --paths. Only set this if --paths already lists every package a call path might route through; otherwise chains through an unlisted shared/internal package will silently look like a dead end.")
	liveCmd.PersistentFlags().StringVar(&liveHost, "host", "127.0.0.1", "Address to bind the live UI to. Defaults to localhost only -- this exposes source layout and code positions, don't bind it to a public interface.")
	liveCmd.PersistentFlags().IntVarP(&livePort, "port", "P", 1985, "Port for the live UI (distinct from wally server's default 1984, so both can run side by side)")
}

func runLive(cmd *cobra.Command, args []string) {
	nav := navigator.NewNavigator(verbose, nil)
	nav.RunSSA = true
	nav.CallgraphAlg = liveCallgraphAlg
	nav.NoAutoDeps = liveNoAutoDeps
	nav.Exclusions = navigator.Exclusions{
		Packages:    liveExcludePkgs,
		PosSuffixes: liveExcludePos,
	}

	buildTime := buildNav(nav, livePaths)

	srv := live.NewServer(nav, live.Info{
		Paths:        livePaths,
		CallgraphAlg: liveCallgraphAlg,
		BuildTime:    buildTime.String(),
		PackageCount: len(nav.Packages),
	})

	addr := fmt.Sprintf("%s:%d", liveHost, livePort)
	fmt.Printf("wally live UI: http://%s\n", addr)
	log.Fatal(srv.ListenAndServe(addr))
}
