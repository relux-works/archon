// Command harnessgate verifies the explicit historical exception predecessor.
package main

import (
	"context"
	"os"

	"github.com/relux-works/archon/internal/harnessboundary"
)

func main() {
	root, err := os.Getwd()
	if err != nil {
		os.Exit(harnessboundary.RatchetMissingExit)
	}
	os.Exit(harnessboundary.RunRatchetCLI(context.Background(), os.Args[1:], root, os.Getenv, os.Stdout, os.Stderr))
}
