package cmd

import (
	"fmt"
	"github.com/spf13/cobra"
	"strings"
	"wally/indicator"
	"wally/navigator"
	"wally/reporter"
	"wally/server"
	"wally/wallylib/callmapper"
)

var (
	pkg      string
	function string
	recvType string
)

// funcCmd represents the map command
var funcCmd = &cobra.Command{
	Use:   "search",
	Short: "Map a single function",
	Long:  `Performs analysis given a single function"`,
	Run:   searchFunc,
	Args: func(cmd *cobra.Command, args []string) error {
		if format != "" && format != "json" {
			return fmt.Errorf("invalid output type: %q\n", format)
		}

		searchAlg = strings.ToLower(searchAlg)
		if searchAlg != "bfs" && searchAlg != "dfs" {
			return fmt.Errorf("search agorithm should be either bfs or dfs, got %s\n", searchAlg)
		}

		if searchAlg == "dfs" && limiterMode >= 3 || searchAlg == "dfs" && skipClosures {
			return fmt.Errorf("limiter mode 3 or --skip-closure not supported by DFS")
		}

		if callgraphAlg != "rta" && callgraphAlg != "cha" && callgraphAlg != "vta" && callgraphAlg != "static" {
			return fmt.Errorf("callgraph agorithm should be either cha, rta, or vta, got %s\n", callgraphAlg)
		}

		if limiterMode > 3 {
			return fmt.Errorf("limiter-mode should be less than 4, got %d\n", limiterMode)
		}
		return nil
	},
}

func init() {
	mapCmd.AddCommand(funcCmd)
	funcCmd.PersistentFlags().StringVar(&pkg, "pkg", "", "Package name")
	funcCmd.PersistentFlags().StringVar(&function, "func", "", "Function name")
	funcCmd.PersistentFlags().StringVar(&recvType, "recv-type", "", "receiver type name (excluding package)")
	funcCmd.MarkPersistentFlagRequired("pkg")
	funcCmd.MarkPersistentFlagRequired("func")
}

func searchFunc(cmd *cobra.Command, args []string) {
	indicators := indicator.InitIndicators(
		[]indicator.Indicator{
			indicator.Indicator{
				Package:      pkg,
				Function:     function,
				ReceiverType: recvType,
			},
		}, true,
	)

	nav := navigator.NewNavigator(verbose, indicators)
	nav.RunSSA = true
	nav.CallgraphAlg = callgraphAlg

	mapperOptions := callmapper.Options{
		Filter:       filter,
		MaxFuncs:     maxFuncs,
		MaxPaths:     maxPaths,
		PrintNodes:   printNodes,
		Limiter:      callmapper.LimiterMode(limiterMode),
		SearchAlg:    callmapper.SearchAlgs[searchAlg],
		SkipClosures: skipClosures,
	}

	nav.Logger.Info("Running mapper", "indicators", len(indicators))
	nav.MapRoutes(paths)

	nav.Logger.Info("Solving call paths")
	nav.SolveCallPaths(mapperOptions)

	nav.PrintResults(format, outputFile)

	if graph != "" {
		nav.Logger.Info("Generating graph", "graph filename", graph)
		reporter.GenerateGraph(nav.RouteMatches, graph)
	}

	if serverGraph {
		server.ServerCosmograph(reporter.GetJson(nav.RouteMatches), 1984)
	}

}
