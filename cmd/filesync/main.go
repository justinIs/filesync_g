package main

import (
	"flag"
	"fmt"
	"os"
	"path/filepath"

	"github.com/justinIs/filesync_g/internal/config"
	"github.com/justinIs/filesync_g/internal/scan"
)

func main() {
	var source string
	flag.StringVar(&source, "source", config.DefaultSource, "directory to sync (defaults to the current directory)")
	flag.Parse()

	fmt.Println("Running filesync")

	source, err := resolveSource(source)
	if err != nil {
		fmt.Fprintf(os.Stderr, "filesync: %v\n", err)
		os.Exit(1)
	}

	fmt.Printf("Loading config file for source: %v\n", source)

	cfg, err := config.Load(source)
	if err != nil {
		fmt.Fprintf(os.Stderr, "filesync: %v\n", err)
		os.Exit(1)
	}

	fmt.Printf("Config file loaded: %+v\n", cfg)

	entries, err := scan.Scan(source, cfg)
	if err != nil {
		fmt.Fprintf(os.Stderr, "filesync: %v\n", err)
		os.Exit(1)
	}

	fmt.Printf("Found entries: %+v\n", entries)
}

func resolveSource(source string) (string, error) {
	abs, err := filepath.Abs(source)
	if err != nil {
		return "", fmt.Errorf("resolve source: %v", err)
	}
	if info, err := os.Stat(abs); err != nil || !info.IsDir() {
		if err != nil {
			return "", fmt.Errorf("source %q: %w", abs, err)
		} else {
			return "", fmt.Errorf("source %q is not a directory", abs)
		}
	}
	return abs, nil
}
