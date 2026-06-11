package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"os"
	"os/signal"
	"syscall"

	"github.com/justinIs/filesync_g/internal/config"
	"github.com/justinIs/filesync_g/internal/sync"
)

func main() {
	var source string
	var verbose bool
	flag.StringVar(&source, "source", config.DefaultSource, "directory to sync (defaults to the current directory)")
	flag.BoolVar(&verbose, "v", false, "print per-file scan and change tables")
	flag.Parse()

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	if err := sync.Run(ctx, source, verbose); err != nil {
		if errors.Is(err, context.Canceled) {
			os.Exit(130) // interrupted
		}
		fmt.Fprintf(os.Stderr, "filesync: error running: %v", err)
		os.Exit(1)
	}
}
