package cmd

import (
	"os"
	"wally/indicator"

	"github.com/spf13/cobra"
)

type WallyConfig struct {
	Indicators []indicator.Indicator `yaml:"indicators"`
}

var (
	verbose int
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

func init() {
	rootCmd.PersistentFlags().CountVarP(&verbose, "verbose", "v", "verbose output. Up to -vvv levels of verbosity are supported")
}
