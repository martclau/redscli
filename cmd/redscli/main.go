package main

import (
	"os"

	"redscli/internal/redscli"
)

// main delegates to the internal package so command behavior can be tested
// without spawning a separate process.
func main() {
	os.Exit(redscli.Main(os.Args[1:], os.Stdin, os.Stdout, os.Stderr))
}
