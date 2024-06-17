package cmd

import (
	"fmt"
	"github.com/spf13/cobra"
	"gopkg.in/yaml.v2"
	"log"
	"os"
	"strings"
	"wally/indicator"
	"wally/navigator"
	"wally/reporter"
	"wally/server"
	"wally/wallylib/callmapper"
)

var (
	config      string
	wallyConfig WallyConfig

	paths             []string
	runSSA            bool
	filter            string
	graph             string
	maxFuncs          int
	maxPaths          int
	printNodes        bool
	format            string
	outputFile        string
	serverGraph       bool
	skipDefault       bool
	continueAfterMain bool
	searchAlg         string
	callgraphAlg      string
)

// mapCmd represents the map command
var mapCmd = &cobra.Command{
	Use:   "map",
	Short: "Get list a list of all routes",
	Long:  `Get list a list of all routes with resolved values as possible for params, along with enclosing functions"`,
	Args: func(cmd *cobra.Command, args []string) error {
		if format != "" && format != "json" {
			return fmt.Errorf("invalid output type: %q\n", format)
		}

		searchAlg = strings.ToLower(searchAlg)
		if searchAlg != "bfs" && searchAlg != "dfs" {
			return fmt.Errorf("search agorithm should be either bfs or dfs, got %s\n", searchAlg)
		}

		if callgraphAlg != "rta" && callgraphAlg != "cha" && callgraphAlg != "vta" && callgraphAlg != "static" {
			return fmt.Errorf("callgraph agorithm should be either cha, rta, or vta, got %s\n", callgraphAlg)
		}

		return nil
	},
	Run: mapRoutes,
}

func init() {
	rootCmd.AddCommand(mapCmd)

	mapCmd.PersistentFlags().BoolVar(&skipDefault, "skip-default", false, "whether to skip the default indicators")
	mapCmd.PersistentFlags().BoolVar(&continueAfterMain, "continue-after-main", false, "continue callpath search even after reaching main")
	mapCmd.PersistentFlags().StringVarP(&config, "config", "c", "", "path for config file containing indicators")
	mapCmd.PersistentFlags().StringVar(&callgraphAlg, "callgraph-alg", "cha", "cha || rta || vta")

	mapCmd.PersistentFlags().StringSliceVarP(&paths, "paths", "p", paths, "The comma separated package paths to target. Use ./.. for current directory and subdirectories")
	mapCmd.PersistentFlags().StringVarP(&graph, "graph", "g", "", "Path for optional PNG graph output. Only works with --ssa")
	mapCmd.PersistentFlags().StringVar(&searchAlg, "search-alg", "bfs", "Search algorithm used for mapping callgraph (dfs or bfs)")
	mapCmd.PersistentFlags().BoolVar(&runSSA, "ssa", false, "whether to run some checks using SSA")
	mapCmd.PersistentFlags().StringVarP(&filter, "filter", "f", "", "Filter package for call graph search")
	mapCmd.PersistentFlags().IntVar(&maxFuncs, "max-funcs", 0, "Limit the max number of nodes or functions per call path")
	mapCmd.PersistentFlags().IntVar(&maxPaths, "max-paths", 0, "Max paths per node. This helps when wally encounters recursive calls")
	mapCmd.PersistentFlags().BoolVar(&printNodes, "print-nodes", false, "Print the position of call graph paths rather than node")
	mapCmd.PersistentFlags().StringVar(&format, "format", "", "Output format. Supported: json, csv")
	mapCmd.PersistentFlags().StringVarP(&outputFile, "out", "o", "", "Output to file path")

	mapCmd.PersistentFlags().BoolVar(&serverGraph, "server", false, "Starts a server on port 1984 with output graph")
}

func mapRoutes(cmd *cobra.Command, args []string) {
	initConfig()

	indicators := indicator.InitIndicators(wallyConfig.Indicators, skipDefault)
	nav := navigator.NewNavigator(verbose, indicators)
	nav.RunSSA = runSSA
	nav.CallgraphAlg = callgraphAlg

	nav.Logger.Info("Running mapper", "indicators", len(indicators))

	nav.MapRoutes(paths)
	if runSSA {
		mapperOptions := callmapper.Options{
			Filter:            filter,
			MaxFuncs:          maxFuncs,
			MaxPaths:          maxPaths,
			PrintNodes:        printNodes,
			SearchAlg:         callmapper.SearchAlgs[searchAlg],
			ContinueAfterMain: continueAfterMain,
		}
		nav.Logger.Info("Solving call paths")
		nav.SolveCallPaths(mapperOptions)
	}
	nav.Logger.Info("Printing results")
	nav.PrintResults(format, outputFile)

	if runSSA && graph != "" {
		nav.Logger.Info("Generating graph", "graph filename", graph)
		reporter.GenerateGraph(nav.RouteMatches, graph)
	}

	if serverGraph {
		server.ServerCosmograph(reporter.GetJson(nav.RouteMatches), 1984)
	}
}

func initConfig() {
	wallyConfig = WallyConfig{}
	fmt.Println("Looking for config file in ", config)
	if _, err := os.Stat(config); os.IsNotExist(err) {
		fmt.Println("Configuration file `%s` not found. Will run stock indicators only", config)
	} else {
		data, err := os.ReadFile(config)
		if err != nil {
			log.Fatal(err)
		}

		err = yaml.Unmarshal([]byte(data), &wallyConfig)
		if err != nil {
			fmt.Println("Could not load configuration file: %s. Will run stock indicators only", err)
		}
	}
}
