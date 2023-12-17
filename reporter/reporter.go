package reporter

import (
	"bytes"
	"fmt"
	"github.com/goccy/go-graphviz"
	"github.com/goccy/go-graphviz/cgraph"
	"log"
	"wally/wallylib"
)

func PrintResults(matches []wallylib.RouteMatch) {
	for _, match := range matches {
		// TODO: This is printing the values from the indicator
		// That's fine, and it works but it should print values
		// from those captured during navigator, just in case
		PrintMach(match)
	}
	fmt.Println("Total Results: ", len(matches))
}

func PrintMach(match wallylib.RouteMatch) {
	fmt.Println("===========MATCH===============")
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
	fmt.Println("Enclosed by: ", match.EnclosedBy)
	fmt.Printf("Position %s:%d\n", match.Pos.Filename, match.Pos.Line)
	if match.SSA != nil && match.SSA.CallPaths != nil && len(match.SSA.CallPaths) > 0 {
		fmt.Println("Possible Paths:", len(match.SSA.CallPaths))
		for i, paths := range match.SSA.CallPaths {
			fmt.Printf("	Path %d:\n", i+1)
			for x := len(paths) - 1; x >= 0; x-- {
				fmt.Printf("		%s --->\n", paths[x])
			}
		}
	}
	fmt.Println()
}

func GenerateGraph(matches []wallylib.RouteMatch, path string) {
	g := graphviz.New()
	graph, err := g.Graph()
	if err != nil {
		log.Fatal(err)
	}
	for _, match := range matches {
		m, err := graph.CreateNode(match.Indicator.Package + "." + match.Indicator.Function)
		if err != nil {
			log.Fatal(err)
		}
		m = m.SetColor("red").SetFillColor("blue").SetShape("diamond")

		for _, paths := range match.SSA.CallPaths {
			var prev *cgraph.Node
			for i := 0; i < len(paths); i++ {
				if i == 0 {
					prev, err = graph.CreateNode(paths[i])
					if err != nil {
						log.Fatal(err)
					}
					_, err = graph.CreateEdge("e", prev, m)
					if err != nil {
						log.Fatal(err)
					}
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

	var buf bytes.Buffer
	if err := g.Render(graph, graphviz.PNG, &buf); err != nil {
		log.Fatal(err)
	}

	if err := g.RenderFilename(graph, graphviz.PNG, path); err != nil {
		log.Fatal(err)
	}
}
