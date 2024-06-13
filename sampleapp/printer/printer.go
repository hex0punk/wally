package printer

import "fmt"

func PrintOrPanic(word string, idx int) {
	fmt.Println("called")
	letter := word[idx]
	fmt.Println("letter is ", letter)
}
