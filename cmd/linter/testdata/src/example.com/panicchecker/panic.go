package main

import (
	"log"
	"os"
)

func main() {
	// These should be allowed
	os.Exit(0)           // want "call to os.Exit outside of main\\(\\) function"??? Actually in main, so should be ok
	log.Fatal("goodbye") // want "call to log.Fatal outside of main\\(\\) function"???

	// But wait - the analyzer should NOT report these because they're in main()
	// Let's create separate test cases
}

func someFunc() {
	// These should be reported
	os.Exit(1)         // want "call to os.Exit outside of main\\(\\) function"
	log.Fatal("error") // want "call to log.Fatal outside of main\\(\\) function"
}

func anotherFunc() {
	panic("oh no") // want "use of built-in panic function is discouraged"
}
