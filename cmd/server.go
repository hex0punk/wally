package cmd

import (
	"fmt"
	"github.com/hex0punk/wally/server"
	"github.com/spf13/cobra"
	"log"
	"os"
)

var (
	jsonPath string
	port     int
)

var serverCmd = &cobra.Command{
	Use:   "server",
	Short: "Runs the wally server for exploring wally output",
	Long:  `Runs the wally server for exploring wally output`,
	Run:   serve,
}

func init() {
	rootCmd.AddCommand(serverCmd)
	serverCmd.PersistentFlags().StringVarP(&jsonPath, "json-path", "p", "", "Path for json file to visualize")
	serverCmd.PersistentFlags().IntVarP(&port, "port", "P", 1984, "Port number for wally server")
}

func serve(cmd *cobra.Command, args []string) {
	fmt.Println("Looking for wally file in ", jsonPath)
	if _, err := os.Stat(jsonPath); os.IsNotExist(err) {
		log.Fatalf("Wally file `%s` not found\n.", jsonPath)
	}

	data, err := os.ReadFile(jsonPath)
	if err != nil {
		log.Fatal(err)
	}
	server.ServerCosmograph(data, port)
}
