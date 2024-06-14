package printer

import "fmt"

func PrintOrPanic(word string, idx int) {
	letter := word[idx]
	fmt.Println("letter is ", letter)
}
