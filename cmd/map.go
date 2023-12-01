/*
Copyright © 2023 NAME HERE <EMAIL ADDRESS>
*/
package cmd

import (
	"github.com/spf13/cobra"
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
	indicators := indicator.InitIndicators()
	navigator := internal.NewNavigator(verbose, indicators)
	navigator.MapRoutes()
	navigator.PrintResults()
}
