package helper

import (
	"log"
	"os"
)

func DoSomething() {
	// These should be reported (not in main package)
	os.Exit(1)       // want "call to os.Exit outside of main package"
	log.Fatal("bad") // want "call to log.Fatal outside of main package"
	panic("no")      // want "use of built-in panic function is discouraged"
}

func DoSomethingElse() {
	// This should also be reported
	panic("no way") // want "use of built-in panic function is discouraged"
}
