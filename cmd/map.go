package cmd

import (
	"github.com/spf13/cobra"
	"wally/indicator"
	"wally/navigator"
	"wally/reporter"
)

var (
	paths   []string
	runSSA  bool
	filter  string
	graph   string
	limiter int
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
}

func mapRoutes(cmd *cobra.Command, args []string) {
	indicators := indicator.InitIndicators(wallyConfig.Indicators)
	navigator := navigator.NewNavigator(verbose, indicators)
	navigator.RunSSA = runSSA

	navigator.Logger.Info("Running mapper", "indicators", len(indicators))
	navigator.MapRoutes(paths)
	if runSSA {
		navigator.Logger.Info("Solving call paths")
		navigator.SolveCallPaths(filter, limiter)
	}
	navigator.Logger.Info("Printing results")
	navigator.PrintResults()

	if runSSA && graph != "" {
		navigator.Logger.Info("Generating graph", "graph filename", graph)
		reporter.GenerateGraph(navigator.RouteMatches, graph)
	}
}
