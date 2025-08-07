package main

import (
	"fmt"
	"os"

	"linktadoru/internal/cmd"
)

// Version information set by build flags
var (
	Version   = "dev"
	BuildTime = "unknown"
)

func main() {
	// Set version information
	cmd.SetVersionInfo(Version, BuildTime)

	if err := cmd.Execute(); err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}
}
