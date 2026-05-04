package main

import (
	"os"

	"redscli/internal/redscli"
)

func main() {
	os.Exit(redscli.Main(os.Args[1:], os.Stdin, os.Stdout, os.Stderr))
}
