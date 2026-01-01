// Package main is the entry point for the rvn CLI tool.
package main

import (
	"os"

	"github.com/ravenscroftj/raven/internal/cli"
)

func main() {
	if err := cli.Execute(); err != nil {
		os.Exit(1)
	}
}
