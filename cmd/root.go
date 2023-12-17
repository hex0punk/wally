package cmd

import (
	"fmt"
	"gopkg.in/yaml.v2"
	"log"
	"os"
	"wally/indicator"

	"github.com/spf13/cobra"
)

type WallyConfig struct {
	Indicators []indicator.Indicator `yaml:"indicators"`
}

var (
	verbose     int
	config      string
	wallyConfig WallyConfig
)

// rootCmd represents the base command when called without any subcommands
var rootCmd = &cobra.Command{
	Use:   "wally",
	Short: "Wally is a cartographer and helps you find and map HTTP and RPC routes in Go code",
	Long: `Wally is a cartographer from Scabb Island. 
           He wears a monacle and claims to have traveled all over the world`,
}

// Execute adds all child commands to the root command and sets flags appropriately.
// This is called by main.main(). It only needs to happen once to the rootCmd.
func Execute() {
	err := rootCmd.Execute()
	if err != nil {
		os.Exit(1)
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

func init() {
	cobra.OnInitialize(initConfig)
	rootCmd.Flags().BoolP("toggle", "t", false, "Help message for toggle")
	rootCmd.PersistentFlags().CountVarP(&verbose, "verbose", "v", "verbose output. Up to -vvv levels of verbosity are supported")
	mapCmd.PersistentFlags().StringVarP(&config, "config", "c", "", "path for config file containing indicators")
}
