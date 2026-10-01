package reporter

import (
	"encoding/csv"
	"encoding/json"
	"fmt"
	"github.com/goccy/go-graphviz"
	"github.com/goccy/go-graphviz/cgraph"
	"github.com/hex0punk/wally/match"
	"log"
	"os"
	"strings"
)

// PrintResults and PrintMach (the plain-text, human-facing reporter) live in
// pretty.go now, styled with pterm -- everything below stays a plain data
// exporter (json/csv/graph), untouched by that.

func GetJson(matches []match.RouteMatch) []byte {
	jsonOutput, err := json.Marshal(matches)
	if err != nil {
		log.Fatal(err)
	}

	return jsonOutput
}

func PrintJson(matches []match.RouteMatch, filename string) error {
	jsonOutput, err := json.Marshal(matches)
	if err != nil {
		fmt.Println(err)
		return err
	}

	if filename != "" {
		file, err := os.Create(filename)
		if err != nil {
			log.Fatal(err)
		}
		defer file.Close()

		// Write data to the file
		_, err = file.Write(jsonOutput)
		if err != nil {
			log.Fatal(err)
		}
	} else {
		fmt.Println(string(jsonOutput))
	}
	return nil
}

func WriteCSVFile(matches []match.RouteMatch, filePath string) error {
	file, err := os.Create(filePath)
	if err != nil {
		return fmt.Errorf("error creating file: %v", err)
	}
	defer file.Close()

	writer := csv.NewWriter(file)
	defer writer.Flush()

	// Writing the header of the CSV file
	if err := writer.Write([]string{"source", "target"}); err != nil {
		return fmt.Errorf("error writing header to CSV: %v", err)
	}

	for _, match := range matches {
		if match.SSA != nil && match.SSA.CallPaths != nil {
			for _, paths := range match.SSA.CallPaths.Paths {
				for i := 0; i < len(paths.Nodes)-1; i++ {
					if err := writer.Write([]string{paths.Nodes[i].NodeString, paths.Nodes[i+1].NodeString}); err != nil {
						return fmt.Errorf("error writing record to CSV: %v", err)
					}
				}
			}
		}
	}

	return nil
}

// TODO: Move this to a new package dedicated to graphing, or in this same package but in a separate file
func GenerateGraph(matches []match.RouteMatch, path string) {
	g := graphviz.New()
	graph, err := g.Graph()
	if err != nil {
		log.Fatal(err)
	}
	for _, match := range matches {
		for _, paths := range match.SSA.CallPaths.Paths {
			var prev *cgraph.Node
			for i := 0; i < len(paths.Nodes); i++ {
				if i == 0 {
					prev, err = graph.CreateNode(paths.Nodes[i].NodeString)
					if err != nil {
						log.Fatal(err)
					}
					prev = prev.SetColor("red").SetFillColor("blue").SetShape("diamond")
				} else {
					newNode, err := graph.CreateNode(paths.Nodes[i].NodeString)
					if err != nil {
						log.Fatal(err)
					}
					_, err = graph.CreateEdge("e", newNode, prev)
					if err != nil {
						log.Fatal(err)
					}
					prev = newNode
				}
			}
		}
	}

	if strings.HasSuffix(path, ".png") {
		if err := g.RenderFilename(graph, graphviz.PNG, path); err != nil {
			log.Fatal(err)
		}
	} else if strings.HasSuffix(path, ".xdot") {
		if err := g.RenderFilename(graph, graphviz.XDOT, path); err != nil {
			log.Fatal(err)
		}
	}
}
