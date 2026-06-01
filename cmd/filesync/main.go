package main

import (
	"fmt"
	"os"

	"github.com/justinIs/filesync_g/internal/config"
	"github.com/justinIs/filesync_g/internal/scan"
)

func main() {
	fmt.Println("Running filesync")

	config, err := config.Load("filesync.toml")
	if err != nil {
		fmt.Fprintf(os.Stderr, "filesync: %v\n", err)
		os.Exit(1)
	}

	fmt.Printf("Config file loaded: %v\n", config)

	entries, err := scan.Scan(".", config)
	if err != nil {
		fmt.Fprintf(os.Stderr, "filesync: %v\n", err)
		os.Exit(1)
	}

	fmt.Printf("Found entries: %v", entries)
}
