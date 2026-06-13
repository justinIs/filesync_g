// Package sync orchestrates the syncing process
package sync

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/justinIs/filesync_g/internal/config"
	"github.com/justinIs/filesync_g/internal/scan"
	"github.com/justinIs/filesync_g/internal/store"
	"github.com/justinIs/filesync_g/internal/track"
)

func Run(ctx context.Context, source string, verbose bool, deleteFiles bool) (err error) {
	if verbose {
		fmt.Printf("sync#Run source: %s, deleteFiles: %v", source, deleteFiles)
	}
	source, err = resolveSource(source)
	if err != nil {
		return err
	}

	fmt.Printf("sync: scanning %s\n", source)

	cfg, err := config.Load(source)
	if err != nil {
		return err
	}
	if verbose {
		printConfig(os.Stdout, cfg)
	}

	// Scan for entries on local filesystem
	entries, err := scan.Scan(ctx, source, cfg)
	if err != nil {
		return err
	}

	// Load the manifest to get the current state
	manifest, err := track.LoadManifest(source)
	if err != nil {
		return err
	}

	// Check entries against the manifest for changes
	results, err := manifest.CheckEntries(ctx, entries)
	if err != nil {
		return err
	}

	if verbose {
		printEntries(os.Stdout, entries)
		printResults(os.Stdout, results)
	} else {
		printSummary(os.Stdout, len(entries), results)
	}

	// Create the committer to update the manifest (state) for when changes are made
	committer := track.NewCommitter(manifest)
	defer func() {
		if cerr := committer.Close(); cerr != nil && err == nil {
			err = cerr
		} else if err == nil {
			fmt.Println("Successfully synced manifest")
		} else {
			fmt.Printf("Received error on manifest sync: %v", err)
		}
	}()

	// TODO: file store should be backend agnostic
	fileStore, err := store.NewS3FileStore(
		ctx,
		store.S3FileStoreConfig(cfg.Store),
	)
	if err != nil {
		return err
	}

	// Apply refreshes to manifest right away
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

	if len(results.Updates) == 0 && (!deleteFiles || len(results.Deletes) == 0) {
		fmt.Println("No files to sync with backend")
		return nil
	}

	// TODO: concurrency

	type uploadResults struct {
		uploadCount int
		failedCount int
	}
	fileUploadResults := uploadResults{}
	for _, u := range results.Updates {
		if ctx.Err() != nil {
			break // user interrupted - break to just flush manifest
		}
		canceled, err := uploadFile(ctx, fileStore, source, u)
		if canceled {
			// context canceled - user interrupted
			break
		}
		if err != nil {
			perror("error uploading file: %v", err)
			fileUploadResults.failedCount++
			continue
		}
		fileUploadResults.uploadCount++
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
	if deleteFiles &&
		len(results.Deletes) > 0 &&
		confirm(fmt.Sprintf("%d files to delete, are you sure?", len(results.Deletes))) {
		for _, d := range results.Deletes {
			if ctx.Err() != nil {
				break
			}
			if err := fileStore.Delete(ctx, d.RelPath); err != nil {
				if errors.Is(err, context.Canceled) {
					break
				}
				perror("error deleting file: %v", err)
				continue
			}
			committer.Send(track.SyncOutcome{
				Info: track.ManifestFileInfo{
					RelPath: d.RelPath,
					Size:    d.Size,
					ModTime: d.ModTime,
					Hash:    d.Hash,
				},
				Op: track.OpKindDelete,
			})
		}
	}
	if ctx.Err() != nil {
		return ctx.Err()
	}

	fmt.Printf(
		"scan: file upload results\nuploaded: %d\nfailed: %d\n",
		fileUploadResults.uploadCount,
		fileUploadResults.failedCount,
	)

	return nil
}

func uploadFile(ctx context.Context, fileStore store.FileStore, source string, fi track.ManifestFileInfo) (canceled bool, err error) {
	f, err := os.Open(filepath.Join(source, fi.RelPath))
	if err != nil {
		return false, err
	}
	defer func() { _ = f.Close() }()
	if err := fileStore.Put(ctx, fi.RelPath, f); err != nil {
		if errors.Is(err, context.Canceled) {
			return true, nil
		}
		return false, err
	}
	return false, nil
}

func confirm(prompt string) bool {
	fmt.Fprintf(os.Stderr, "%s [y/N]", prompt)

	reader := bufio.NewReader(os.Stdin)
	line, err := reader.ReadString('\n')
	if err != nil {
		if errors.Is(err, io.EOF) {
			return false
		}
		fmt.Fprintf(os.Stderr, "Error reading confirm input: %v", err)
		return false
	}

	switch strings.ToLower(strings.TrimSpace(line)) {
	case "y", "yes":
		return true
	default:
		return false
	}
}

// perror prints a prefixed error to stderr
func perror(format string, args ...any) {
	fmt.Fprintf(os.Stderr, "sync: "+format+"\n", args...)
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
