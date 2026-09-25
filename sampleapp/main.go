package main

import (
	"fmt"
	"github.com/hex0punk/wally/sampleapp/bound"
	"github.com/hex0punk/wally/sampleapp/caller"
	"github.com/hex0punk/wally/sampleapp/printer"
	"github.com/hex0punk/wally/sampleapp/safe"
	"github.com/hex0punk/wally/sampleapp/target"
)

func main() {
	word := "Hello"
	idx := 7
	printCharSafe(word, idx)
	printChar(word, idx)
	RunAll(word, idx)
	ra := RunAll
	ra(word, idx)
	RunBound()
	RunCrossInterface()
}

// RunBound exercises a bound method value: h.Handle is not called directly,
// it is taken as a func value ($bound synthetic wrapper) and invoked later
// via Dispatch, one level removed from its creation site.
func RunBound() {
	h := &bound.Handler{Name: "h1"}
	Dispatch(h.Handle, "hello")
}

func Dispatch(f func(string), msg string) {
	f(msg)
}

// RunCrossInterface exercises a call dispatched through an interface
// declared in a different package than the concrete type actually
// implementing it: c is a *target.Client, but caller.RunWithWorker's own
// parameter type is caller.Worker, declared in caller's own package with
// no reference to target at all. A search for target.Client.DoWork by
// exact receiver-type string can't see this call; it needs interface
// satisfaction (does target.Client implement whatever interface the call
// site's own receiver type actually is), not string equality.
func RunCrossInterface() {
	c := &target.Client{}
	caller.RunWithWorker(c, 1)
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
