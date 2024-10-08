package main

import (
	"fmt"
	"github.com/hex0punk/wally/sampleapp/printer"
	"github.com/hex0punk/wally/sampleapp/safe"
)

func main() {
	word := "Hello"
	idx := 7
	printCharSafe(word, idx)
	printChar(word, idx)
	RunAll(word, idx)
	ra := RunAll
	ra(word, idx)
}

func RunAll(str string, idx int) {
	printCharSafe(str, idx)
	printer.PrintOrPanic(str, idx)
	testF := printCharSafe
	testF(str, idx)
}

func ThisIsACall(str string) {
	fmt.Println(str)
}
func printCharSafe(word string, idx int) {
	safe.RunSafely(
		func() {
			printer.PrintOrPanic(word, idx)
		})
}

func printChar(word string, idx int) {
	ThisIsACall("HOOOOLA")
	//printer.PrintOrPanic(word, idx)
}
