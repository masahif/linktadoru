package main

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"syscall"

	"github.com/masahif/linktadoru/internal/cmd"
)

// Version information set by build flags
var (
	Version   = "dev"
	BuildTime = "unknown"
)

func main() {
	// Set version information
	cmd.SetVersionInfo(Version, BuildTime)

	// Cancel the crawl context on SIGINT/SIGTERM so workers stop cleanly and
	// the database is closed, instead of the process being killed mid-write.
	// Signals stay captured until stop() runs (after ExecuteContext returns),
	// so repeated Ctrl-C during shutdown is absorbed rather than fatal.
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	if err := cmd.ExecuteContext(ctx); err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}
}
