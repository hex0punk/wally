package cmd

import (
	"github.com/spf13/cobra"
	"wally/indicator"
	"wally/navigator"
	"wally/reporter"
)

var (
	path   string
	runSSA bool
	filter string
	graph  string
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
	mapCmd.PersistentFlags().StringVarP(&path, "path", "p", "", "The package to target")
	mapCmd.PersistentFlags().StringVarP(&graph, "graph", "g", "", "Path for optional PNG graph output. Only works with --ssa")
	mapCmd.PersistentFlags().BoolVar(&runSSA, "ssa", false, "whether to run some checks using SSA")
	mapCmd.PersistentFlags().StringVarP(&filter, "filter", "f", "", "Filter package for call graph search")
}

func mapRoutes(cmd *cobra.Command, args []string) {
	indicators := indicator.InitIndicators(wallyConfig.Indicators)
	navigator := navigator.NewNavigator(verbose, indicators)
	navigator.RunSSA = runSSA

	navigator.Logger.Info("Running mapper", "indicators", len(indicators))
	navigator.MapRoutes(path)
	if runSSA {
		navigator.Logger.Info("Solving call paths")
		navigator.SolveCallPaths(filter)
	}
	navigator.Logger.Info("Printing results")
	navigator.PrintResults()

	if runSSA && graph != "" {
		navigator.Logger.Info("Generating graph", "graph filename", graph)
		reporter.GenerateGraph(navigator.RouteMatches, graph)
	}

}
