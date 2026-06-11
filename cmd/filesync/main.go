package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"path/filepath"

	"github.com/justinIs/filesync_g/internal/config"
	"github.com/justinIs/filesync_g/internal/scan"
	"github.com/justinIs/filesync_g/internal/store"
	"github.com/justinIs/filesync_g/internal/track"
)

func main() {
	var source string
	var verbose bool
	flag.StringVar(&source, "source", config.DefaultSource, "directory to sync (defaults to the current directory)")
	flag.BoolVar(&verbose, "v", false, "print per-file scan and change tables")
	flag.Parse()

	source, err := resolveSource(source)
	if err != nil {
		fatalf("%v", err)
	}

	fmt.Printf("filesync: scanning %s\n", source)

	cfg, err := config.Load(source)
	if err != nil {
		fatalf("%v", err)
	}
	if verbose {
		printConfig(os.Stdout, cfg)
	}

	entries, err := scan.Scan(source, cfg)
	if err != nil {
		fatalf("%v", err)
	}

	manifest, err := track.LoadManifest(source)
	if err != nil {
		fatalf("manifest: %v", err)
	}

	results, err := manifest.CheckEntries(entries)
	if err != nil {
		fatalf("error checking entries: %v", err)
	}

	if verbose {
		printEntries(os.Stdout, entries)
		printResults(os.Stdout, results)
	} else {
		printSummary(os.Stdout, len(entries), results)
	}

	committer := track.NewCommitter(manifest)

	fileStore, err := store.NewS3FileStore(context.Background(), store.S3FileStoreConfig(cfg.Store))
	if err != nil {
		fatalf("error creating fileStore: %v", err)
	}

	for _, r := range results.Refreshes {
		committer.Send(track.SyncOutcome{
			Info: track.ManifestFileInfo{
				RelPath: r.RelPath,
				Size:    r.Size,
				ModTime: r.ModTime,
				Hash:    r.Hash,
			},
			Op: track.OpKindRefresh,
		})
	}

	for _, u := range results.Updates {
		f, err := os.Open(filepath.Join(source, u.RelPath))
		if err != nil {
			fatalf("could not open file for upload %s: %v", u.RelPath, err)
		}
		if err := fileStore.Put(context.Background(), u.RelPath, f); err != nil {
			fatalf("error uploading file %s: %v", u.RelPath, err)
		}
		committer.Send(track.SyncOutcome{
			Info: track.ManifestFileInfo{
				RelPath: u.RelPath,
				Size:    u.Size,
				ModTime: u.ModTime,
				Hash:    u.Hash,
			},
			Op: track.OpKindUpdate,
		})
	}

	if err := committer.Close(); err != nil {
		fatalf("error committing syncs: %v", err)
	} else {
		fmt.Println("Successfully synced files")
	}
}

// fatalf prints a prefixed error to stderr and exits non-zero.
func fatalf(format string, args ...any) {
	fmt.Fprintf(os.Stderr, "filesync: "+format+"\n", args...)
	os.Exit(1)
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
