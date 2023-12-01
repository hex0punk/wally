/*
Copyright © 2023 NAME HERE <EMAIL ADDRESS>
*/
package cmd

import (
	"fmt"
	"github.com/spf13/cobra"
	"golang.org/x/tools/go/analysis"
	"golang.org/x/tools/go/analysis/passes/inspect"
	"wally/indicator"
	"wally/internal"
)

var (
	pkg string
)

// mapCmd represents the map command
var mapCmd = &cobra.Command{
	Use:   "map",
	Short: "A brief description of your command",
	Long: `A longer description that spans multiple lines and likely contains examples
		and usage of using your command. For example:
		Cobra is a CLI library for Go that empowers applications.
		This application is a tool to generate the needed files
		to quickly create a Cobra application.`,
	Run: mapRoutes,
}

func init() {
	rootCmd.AddCommand(mapCmd)
	mapCmd.PersistentFlags().StringVarP(&pkg, "path", "p", "", "The package to target")
}

func mapRoutes(cmd *cobra.Command, args []string) {
	WithAnalyzer()
	//indicators := indicator.InitIndicators()
	//navigator := internal.NewNavigator(verbose, indicators)
	//navigator.MapRoutes()
	//navigator.PrintResults()
}

func WithAnalyzer() {
	indicators := indicator.InitIndicators()
	navigator := internal.NewNavigator(verbose, indicators)
	pkgs := internal.LoadPackagesForAnalyzer("./...")

	var analyzer = &analysis.Analyzer{
		Name:     "addlint",
		Doc:      "reports integer additions",
		Run:      navigator.Run,
		Requires: []*analysis.Analyzer{inspect.Analyzer},
	}
	results := map[*analysis.Analyzer]interface{}{}
	for _, pkg := range pkgs {
		pass := &analysis.Pass{
			Analyzer:          analyzer,
			Fset:              pkg.Fset,
			Files:             pkg.Syntax,
			OtherFiles:        pkg.OtherFiles,
			IgnoredFiles:      pkg.IgnoredFiles,
			Pkg:               pkg.Types,
			TypesInfo:         pkg.TypesInfo,
			TypesSizes:        pkg.TypesSizes,
			ResultOf:          results,
			Report:            func(d analysis.Diagnostic) {},
			ImportObjectFact:  nil,
			ExportObjectFact:  nil,
			ImportPackageFact: nil,
			ExportPackageFact: nil,
			AllObjectFacts:    nil,
			AllPackageFacts:   nil,
		}

		res, err := inspect.Analyzer.Run(pass)
		if err != nil {
			fmt.Printf(err.Error())
			continue
		}

		pass.ResultOf[inspect.Analyzer] = res

		result, err := pass.Analyzer.Run(pass)
		if err != nil {
			fmt.Printf("Error running analyzer %s: %s\n", analyzer.Name, err)
			continue
		}
		// This should be placed outside of this loop
		// we want to collect single results here, then run through all at the end.
		if result != nil {
			//fmt.Println("printing results")
			if passIssues, ok := result.([]internal.RouteMatch); ok {
				for _, iss := range passIssues {
					navigator.RouteMatches = append(navigator.RouteMatches, iss)
				}
			}
		}
	}
	navigator.PrintResults()
}
