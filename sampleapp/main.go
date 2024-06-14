package main

import (
	"github.com/hex0punk/wally/sampleapp/printer"
	"github.com/hex0punk/wally/sampleapp/safe"
)

func main() {
	word := "Hello"
	idx := 7
	printCharSafe(word, idx)
	printChar(word, idx)
}

func printCharSafe(word string, idx int) {
	safe.RunSafely(func() {
		printer.PrintOrPanic(word, idx)
	})
}

func printChar(word string, idx int) {
	printer.PrintOrPanic(word, idx)
}
