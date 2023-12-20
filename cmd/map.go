package cmd

import (
	"github.com/spf13/cobra"
	"wally/indicator"
	"wally/navigator"
	"wally/reporter"
	"wally/wallylib/callmapper"
)

var (
	paths      []string
	runSSA     bool
	filter     string
	graph      string
	limiter    int
	printNodes bool
)

// mapCmd represents the map command
var mapCmd = &cobra.Command{
	Use:   "map",
	Short: "Get list a list of all routes",
	Long:  `Get list a list of all routes with resolved values as possible for params, along with enclosing functions"`,
	Run:   mapRoutes,
}

func init() {
	rootCmd.AddCommand(mapCmd)
	mapCmd.PersistentFlags().StringSliceVarP(&paths, "paths", "p", paths, "The comma separated package paths to target. Use ./.. for current directory and subdirectories")
	mapCmd.PersistentFlags().StringVarP(&graph, "graph", "g", "", "Path for optional PNG graph output. Only works with --ssa")
	mapCmd.PersistentFlags().BoolVar(&runSSA, "ssa", false, "whether to run some checks using SSA")
	mapCmd.PersistentFlags().StringVarP(&filter, "filter", "f", "", "Filter package for call graph search")
	mapCmd.PersistentFlags().IntVarP(&limiter, "rec-limit", "l", 0, "Limit the max number of recursive calls wally makes when mapping call stacks")
	mapCmd.PersistentFlags().BoolVar(&printNodes, "print-nodes", false, "Print the position of call graph paths rather than node")
}

func mapRoutes(cmd *cobra.Command, args []string) {
	indicators := indicator.InitIndicators(wallyConfig.Indicators)
	nav := navigator.NewNavigator(verbose, indicators)
	nav.RunSSA = runSSA

	nav.Logger.Info("Running mapper", "indicators", len(indicators))
	nav.MapRoutes(paths)
	if runSSA {
		mapperOptions := callmapper.Options{
			Filter:     filter,
			RecLimit:   limiter,
			PrintNodes: printNodes,
		}
		nav.Logger.Info("Solving call paths")
		nav.SolveCallPaths(mapperOptions)
	}
	nav.Logger.Info("Printing results")
	nav.PrintResults()

	if runSSA && graph != "" {
		nav.Logger.Info("Generating graph", "graph filename", graph)
		reporter.GenerateGraph(nav.RouteMatches, graph)
	}
}
