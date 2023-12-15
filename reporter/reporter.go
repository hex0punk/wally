package reporter

import (
	"fmt"
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
