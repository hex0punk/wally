package reporter

import (
	"encoding/csv"
	"encoding/json"
	"fmt"
	"github.com/goccy/go-graphviz"
	"github.com/goccy/go-graphviz/cgraph"
	"log"
	"os"
	"strings"
	"wally/match"
)

func PrintResults(matches []match.RouteMatch) {
	for _, match := range matches {
		// TODO: This is printing the values from the indicator
		// That's fine, and it works but it should print values
		// from those captured during navigator, just in case
		PrintMach(match)
	}
	fmt.Println("Total Results: ", len(matches))
}

func PrintMach(match match.RouteMatch) {
	fmt.Println("===========MATCH===============")
	fmt.Println("ID: ", match.MatchId)
	fmt.Println("Indicator ID: ", match.Indicator.Id)
	fmt.Println("Package: ", match.Indicator.Package)
	fmt.Println("Function: ", match.Indicator.Function)
	fmt.Println("Params: ")
	for k, v := range match.Params {
		if v == "" {
			v = "<could not resolve>"
		}
		if k == "" {
			k = "<not specified>"
		}
		fmt.Printf("	%s: %s\n", k, v)
	}

	if match.SSA != nil && match.SSA.EnclosedByFunc != nil {
		fmt.Println("Enclosed by: ", match.SSA.EnclosedByFunc.String())
	} else {
		fmt.Println("Enclosed by: ", match.EnclosedBy)
	}

	fmt.Printf("Position %s:%d\n", match.Pos.Filename, match.Pos.Line)
	if match.SSA != nil && match.SSA.CallPaths != nil && len(match.SSA.CallPaths) > 0 {
		if match.SSA.RecLimited {
			fmt.Println("Possible Paths (rec limited):", len(match.SSA.CallPaths))
		} else {
			fmt.Println("Possible Paths:", len(match.SSA.CallPaths))
		}

		for i, paths := range match.SSA.CallPaths {
			fmt.Printf("	Path %d:\n", i+1)
			for x := len(paths) - 1; x >= 0; x-- {
				fmt.Printf("		%s --->\n", paths[x])
			}
		}
	}
	fmt.Println()
}

func PrintJson(matches []match.RouteMatch, filename string) error {
	jsonOutput, err := json.Marshal(matches)
	if err != nil {
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
			for _, paths := range match.SSA.CallPaths {
				for i := 0; i < len(paths)-1; i++ {
					if err := writer.Write([]string{paths[i], paths[i+1]}); err != nil {
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
		for _, paths := range match.SSA.CallPaths {
			var prev *cgraph.Node
			for i := 0; i < len(paths); i++ {
				if i == 0 {
					prev, err = graph.CreateNode(paths[i])
					if err != nil {
						log.Fatal(err)
					}
					prev = prev.SetColor("red").SetFillColor("blue").SetShape("diamond")
				} else {
					newNode, err := graph.CreateNode(paths[i])
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
