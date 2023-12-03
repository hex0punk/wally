/*
Copyright © 2023 NAME HERE <EMAIL ADDRESS>
*/
package cmd

import (
	"fmt"
	"github.com/spf13/viper"
	"os"
	"wally/indicator"

	"github.com/spf13/cobra"
)

var (
	verbose int
	config  bool
)

// rootCmd represents the base command when called without any subcommands
var rootCmd = &cobra.Command{
	Use:   "wally",
	Short: "A brief description of your application",
	Long: `A longer description that spans multiple lines and likely contains
examples and usage of using your application. For example:

Cobra is a CLI library for Go that empowers applications.
This application is a tool to generate the needed files
to quickly create a Cobra application.`,
	// Uncomment the following line if your bare application
	// has an action associated with it:
	// Run: func(cmd *cobra.Command, args []string) { },
}

type Config struct {
	Indicators []indicator.Indicator `yaml:"indicators,mapstructure"`
}

// Execute adds all child commands to the root command and sets flags appropriately.
// This is called by main.main(). It only needs to happen once to the rootCmd.
func Execute() {
	fmt.Println("not reading config")
	if config {
		fmt.Println("reading config")
		err := viper.ReadInConfig()
		if err != nil { // Handle errors reading the config file
			panic(fmt.Errorf("fatal error config file: %w", err))
		}

		//indicators := viper.Get("indicators")
		re := viper.Get("indicators")
		fmt.Println(re)
		var indicators []indicator.Indicator
		viper.UnmarshalKey("indicators", &config)
		fmt.Println(indicators[0])

		//if in, ok := indicators.(*[]indicator.Indicator); ok {
		//	fmt.Println("here")
		//	fmt.Println(in)
		//}
	}

	err := rootCmd.Execute()
	if err != nil {
		os.Exit(1)
	}
}

func init() {
	// Here you will define your flags and configuration settings.
	// Cobra supports persistent flags, which, if defined here,
	// will be global for your application.

	// rootCmd.PersistentFlags().StringVar(&cfgFile, "config", "", "config file (default is $HOME/.wally.yaml)")

	// Cobra also supports local flags, which will only run
	// when this action is called directly.
	rootCmd.Flags().BoolP("toggle", "t", false, "Help message for toggle")
	rootCmd.PersistentFlags().CountVarP(&verbose, "verbose", "v", "verbose output. Up to -vvv levels of verbosity are supported")
	//rootCmd.PersistentFlags().BoolVarP(&config, "config", "c", true, "whether to use .wally.yaml")
	//
	//viper.SetDefault("ContentDir", ".")
	//
	//viper.SetConfigName(".wally")
	//viper.SetConfigType("yaml")
	//viper.AddConfigPath(".")
}
