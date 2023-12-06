package cmd

import (
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
	Short: "Wally is a cartographer and helps you find and map HTTP and RPC routes in Go code",
	Long: `Wally is a cartographer from Scabb Island. 
           He wears a monacle and claims to have traveled all over the world`,
}

type Config struct {
	Indicators []indicator.Indicator `yaml:"indicators"`
}

// Execute adds all child commands to the root command and sets flags appropriately.
// This is called by main.main(). It only needs to happen once to the rootCmd.
func Execute() {
	//if config {
	//	fmt.Println("reading config")
	//	err := viper.ReadInConfig()
	//	if err != nil { // Handle errors reading the config file
	//		panic(fmt.Errorf("fatal error config file: %w", err))
	//	}
	//
	//	//re := viper.Get("indicators")
	//	//fmt.Println(re)
	//	var indicators []indicator.Indicator
	//	viper.Unmarshal(&config)
	//	fmt.Println("Indicators")
	//	for _, ind := range indicators {
	//		fmt.Printf("package: %s\n", ind.Package)
	//		fmt.Printf("function: %s\n", ind.Function)
	//		fmt.Println()
	//	}
	//	//in := viper.get
	//	//for _, ind := range in.([]interface{}) {
	//	//	fmt.Printf("package: %s\n", ind.(indicator.Indicator).Package)
	//	//	fmt.Printf("function: %s\n", ind.(indicator.Indicator).Function)
	//	//	fmt.Println()
	//	//}
	//	//fmt.Println(viper.Get("indicators"))
	//}

	err := rootCmd.Execute()
	if err != nil {
		os.Exit(1)
	}
}

func init() {
	rootCmd.Flags().BoolP("toggle", "t", false, "Help message for toggle")
	rootCmd.PersistentFlags().CountVarP(&verbose, "verbose", "v", "verbose output. Up to -vvv levels of verbosity are supported")
	rootCmd.PersistentFlags().BoolVarP(&config, "config", "c", true, "whether to use .wally.yaml")

	viper.SetDefault("ContentDir", ".")

	viper.SetConfigName(".wally")
	viper.SetConfigType("yaml")
	viper.AddConfigPath(".")
}
