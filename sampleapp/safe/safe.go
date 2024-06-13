package safe

import "fmt"

func RunSafely(fn func()) {
	defer func() {
		if recovered := recover(); recovered != nil {
			fmt.Printf("recovered by safe.Wrap - %v\r\n", fn)
			return
		}
	}()
	fn()
}
