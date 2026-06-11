// Package sync orchestrates the syncing process
package sync

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"

	"github.com/justinIs/filesync_g/internal/config"
	"github.com/justinIs/filesync_g/internal/scan"
	"github.com/justinIs/filesync_g/internal/store"
	"github.com/justinIs/filesync_g/internal/track"
)

func Run(ctx context.Context, source string, verbose bool) (err error) {
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

	entries, err := scan.Scan(ctx, source, cfg)
	if err != nil {
		return err
	}

	manifest, err := track.LoadManifest(source)
	if err != nil {
		return err
	}

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

	committer := track.NewCommitter(manifest)
	defer func() {
		if cerr := committer.Close(); cerr != nil && err == nil {
			err = cerr
		} else if err == nil {
			fmt.Println("Successfully synced manifest")
		}
	}()

	fileStore, err := store.NewS3FileStore(
		ctx,
		store.S3FileStoreConfig(cfg.Store),
	)
	if err != nil {
		return err
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

	if len(results.Updates) == 0 {
		fmt.Println("No files to upload")
		return nil
	}

	type uploadReults struct {
		uploadCount int
		failedCount int
	}
	fileUploadResults := uploadReults{}
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
			perror("error uploading file: %w", err)
			return err
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
	defer func() { _ = f.Close() }()
	if err != nil {
		return false, err
	}
	if err := fileStore.Put(ctx, fi.RelPath, f); err != nil {
		if errors.Is(err, context.Canceled) {
			return true, nil
		}
		return false, err
	}
	return false, nil
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
