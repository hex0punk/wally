package cmd

import (
	"github.com/spf13/cobra"
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
		Filter:            filter,
		MaxFuncs:          maxFuncs,
		MaxPaths:          maxPaths,
		PrintNodes:        printNodes,
		ContinueAfterMain: continueAfterMain,
		SearchAlg:         callmapper.SearchAlgs[searchAlg],
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
