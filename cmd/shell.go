package cmd

import (
	"bufio"
	"fmt"
	"github.com/hex0punk/wally/indicator"
	"github.com/hex0punk/wally/navigator"
	"github.com/hex0punk/wally/wallylib/callmapper"
	"github.com/spf13/cobra"
	"github.com/spf13/pflag"
	"gopkg.in/yaml.v2"
	"io"
	"log"
	"os"
	"strings"
	"time"
)

// Session-level flags for `wally shell`. These are set once at startup and
// apply to the whole session: changing paths or the callgraph algorithm
// requires a rebuild (see the `reload` shell command), unlike the
// per-query flags below, which are re-parsed for every line typed at the
// prompt.
var (
	shellPaths        []string
	shellConfig       string
	shellCallgraphAlg string
	shellSkipDefault  bool
	shellExcludePkgs  []string
	shellExcludePos   []string
)

var shellCmd = &cobra.Command{
	Use:   "shell",
	Short: "Build the SSA callgraph once and query it interactively",
	Long: `Building the SSA-based callgraph is the slow part of wally. This command
builds it once, keeps it resident in memory, and then lets you run many
"map search"-style queries against it interactively, without paying the
callgraph-construction cost again for each one.`,
	Args: func(cmd *cobra.Command, args []string) error {
		if shellCallgraphAlg != "rta" && shellCallgraphAlg != "cha" && shellCallgraphAlg != "vta" && shellCallgraphAlg != "static" {
			return fmt.Errorf("callgraph agorithm should be either cha, rta, or vta, got %s", shellCallgraphAlg)
		}
		return nil
	},
	Run: runShell,
}

func init() {
	rootCmd.AddCommand(shellCmd)

	shellCmd.PersistentFlags().StringSliceVarP(&shellPaths, "paths", "p", shellPaths, "The comma separated package paths to target. Use ./... for current directory and subdirectories")
	shellCmd.PersistentFlags().StringVarP(&shellConfig, "config", "c", "", "path for config file containing default indicators to preload on startup")
	shellCmd.PersistentFlags().BoolVar(&shellSkipDefault, "skip-default", true, "whether to skip the built-in http/grpc indicators when preloading --config")
	shellCmd.PersistentFlags().StringVar(&shellCallgraphAlg, "callgraph-alg", "cha", "cha || rta || vta || static")
	shellCmd.PersistentFlags().StringSliceVar(&shellExcludePkgs, "exclude-pkg", []string{}, "Comma separated list of packages to exclude")
	shellCmd.PersistentFlags().StringSliceVar(&shellExcludePos, "exclude-pos", []string{}, "Comma separated list of position suffixes used for filtering the selected function call matches")
}

func runShell(cmd *cobra.Command, args []string) {
	var preloaded []indicator.Indicator
	if shellConfig != "" {
		preloaded = loadShellConfig(shellConfig)
	}

	nav := navigator.NewNavigator(verbose, preloaded)
	nav.RunSSA = true
	nav.CallgraphAlg = shellCallgraphAlg
	nav.Exclusions = navigator.Exclusions{
		Packages:    shellExcludePkgs,
		PosSuffixes: shellExcludePos,
	}

	buildNav(nav)

	fmt.Println()
	fmt.Println("wally interactive shell. The callgraph above was built once; each query below only re-walks it.")
	fmt.Println("Type a query as flags, e.g.: --pkg net/http --func Handle")
	fmt.Println("Other commands: help, reload, exit/quit")
	fmt.Println()

	runShellLoop(nav)
}

func buildNav(nav *navigator.Navigator) {
	start := time.Now()
	nav.Logger.Info("Building SSA callgraph", "paths", shellPaths, "alg", nav.CallgraphAlg)
	nav.Build(shellPaths)
	fmt.Printf("Callgraph built in %s\n", time.Since(start))
}

func runShellLoop(nav *navigator.Navigator) {
	reader := bufio.NewReader(os.Stdin)
	for {
		fmt.Print("wally> ")
		line, err := reader.ReadString('\n')
		if err != nil {
			if err == io.EOF {
				fmt.Println()
				return
			}
			fmt.Fprintln(os.Stderr, "error reading input:", err)
			return
		}

		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}

		switch line {
		case "exit", "quit":
			return
		case "help":
			printShellHelp()
			continue
		case "reload":
			buildNav(nav)
			continue
		}

		runShellQuery(nav, strings.Fields(line))
	}
}

func printShellHelp() {
	fmt.Println(`Query flags (same meaning as "wally map search"):
  --pkg string            Package name (required)
  --func string           Function name (required)
  --recv-type string      Receiver type name (excluding package)
  --match-filter strings  Package prefix(es) used for filtering the selected function call matches
  --filter string         Filter string for call graph search
  --limiter-mode int      Logic level to limit callgraph algorithm spurious nodes (default 4)
  --search-alg string     bfs or dfs (default bfs)
  --max-funcs int         Limit the max number of nodes per call path
  --max-paths int         Max paths per node
  --print-nodes           Print the position of call graph paths rather than node
  --module-only           Filter call paths by the match module (default true)
  --skip-closures         Skip closure edges which can lead to inaccurate results
  --simple                Simple output focuses on function signatures rather than sites
  --format string         Output format. Supported: json, csv
  --out string            Output to file path

Commands:
  help    Show this message
  reload  Rebuild the callgraph from disk (use after editing source)
  exit    Leave the shell (quit also works)`)
}

func runShellQuery(nav *navigator.Navigator, tokens []string) {
	flags := pflag.NewFlagSet("query", pflag.ContinueOnError)

	var (
		qPkg          string
		qFunc         string
		qRecvType     string
		qMatchFilters []string
		qFilter       string
		qLimiterMode  int
		qSearchAlg    string
		qMaxFuncs     int
		qMaxPaths     int
		qPrintNodes   bool
		qModuleOnly   bool
		qSkipClosures bool
		qSimplify     bool
		qFormat       string
		qOut          string
	)

	flags.StringVar(&qPkg, "pkg", "", "Package name")
	flags.StringVar(&qFunc, "func", "", "Function name")
	flags.StringVar(&qRecvType, "recv-type", "", "receiver type name (excluding package)")
	flags.StringSliceVar(&qMatchFilters, "match-filter", []string{}, "Package prefix used for filtering the selected function call matches")
	flags.StringVarP(&qFilter, "filter", "f", "", "Filter string for call graph search")
	flags.IntVar(&qLimiterMode, "limiter-mode", 4, "Logic level to limit callgraph algorithm spurious nodes")
	flags.StringVar(&qSearchAlg, "search-alg", "bfs", "Search algorithm used for mapping callgraph (bfs or dfs)")
	flags.IntVar(&qMaxFuncs, "max-funcs", 0, "Limit the max number of nodes per call path")
	flags.IntVar(&qMaxPaths, "max-paths", 0, "Max paths per node")
	flags.BoolVar(&qPrintNodes, "print-nodes", false, "Print the position of call graph paths rather than node")
	flags.BoolVar(&qModuleOnly, "module-only", true, "Filter call paths by the match module")
	flags.BoolVar(&qSkipClosures, "skip-closures", false, "Skip closure edges which can lead to inaccurate results")
	flags.BoolVarP(&qSimplify, "simple", "s", false, "Simple output focuses on function signatures rather than sites")
	flags.StringVar(&qFormat, "format", "", "Output format. Supported: json, csv")
	flags.StringVarP(&qOut, "out", "o", "", "Output to file path")

	if err := flags.Parse(tokens); err != nil {
		fmt.Println("error:", err)
		return
	}

	if qPkg == "" || qFunc == "" {
		fmt.Println("both --pkg and --func are required (type 'help' for the full flag list)")
		return
	}

	qSearchAlg = strings.ToLower(qSearchAlg)
	if qSearchAlg != "bfs" && qSearchAlg != "dfs" {
		fmt.Printf("search algorithm should be either bfs or dfs, got %s\n", qSearchAlg)
		return
	}

	start := time.Now()

	nav.RouteIndicators = indicator.InitIndicators(
		[]indicator.Indicator{
			{
				Package:      qPkg,
				Function:     qFunc,
				ReceiverType: qRecvType,
				MatchFilters: qMatchFilters,
			},
		}, true,
	)
	nav.RouteMatches = nil
	nav.FindMatches()

	if len(nav.RouteMatches) == 0 {
		fmt.Printf("No matches found for func %s in package %s (resolved in %s)\n", qFunc, qPkg, time.Since(start))
		return
	}

	mapperOptions := callmapper.Options{
		Filter:       qFilter,
		MaxFuncs:     qMaxFuncs,
		MaxPaths:     qMaxPaths,
		PrintNodes:   qPrintNodes,
		Limiter:      callmapper.LimiterMode(qLimiterMode),
		SearchAlg:    callmapper.SearchAlgs[qSearchAlg],
		SkipClosures: qSkipClosures,
		ModuleOnly:   qModuleOnly,
		Simplify:     qSimplify,
	}
	nav.SolveCallPaths(mapperOptions)

	nav.PrintResults(qFormat, qOut)
	fmt.Printf("(resolved in %s against the already-built callgraph)\n", time.Since(start))
}

func loadShellConfig(path string) []indicator.Indicator {
	data, err := os.ReadFile(path)
	if err != nil {
		log.Fatalf("could not read config file %s: %s", path, err)
	}

	var cfg WallyConfig
	if err := yaml.Unmarshal(data, &cfg); err != nil {
		log.Fatalf("could not parse config file %s: %s", path, err)
	}

	return indicator.InitIndicators(cfg.Indicators, shellSkipDefault)
}
