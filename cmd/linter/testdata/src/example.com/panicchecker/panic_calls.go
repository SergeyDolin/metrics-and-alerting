package main

import (
	"log"
	"os"
)

func main() {
	// These should NOT be reported (in main)
	os.Exit(0)
	log.Fatal("in main")
}

func helper() {
	// These SHOULD be reported
	panic("help!")   // want "use of built-in panic function is discouraged"
	os.Exit(1)       // want "call to os.Exit outside of main\\(\\) function"
	log.Fatal("bad") // want "call to log.Fatal outside of main\\(\\) function"
}

func another() {
	// This is also bad
	panic("another") // want "use of built-in panic function is discouraged"
}
