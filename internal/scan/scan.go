// Package scan performs directory walking against the config applying ignoring rules
package scan

import (
	"context"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"time"
)

type Entry struct {
	RelPath string
	Size    int64
	ModTime time.Time
}

type Ignorer interface {
	ShouldIgnore(relPath string, isDir bool) bool
}

func Scan(ctx context.Context, root string, ig Ignorer) ([]Entry, error) {
	var entries []Entry
	if err := filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
		if ctx.Err() != nil {
			return ctx.Err()
		}
		if err != nil {
			if path == root {
				return err
			}

			fmt.Fprintf(os.Stderr, "scan WalkDir error: %v\n", err)
			return nil
		}

		rel, err := filepath.Rel(root, path)
		if err != nil {
			return err
		}
		if rel == "." {
			return nil
		}
		rel = filepath.ToSlash(rel)

		if ig.ShouldIgnore(rel, d.IsDir()) {
			if d.IsDir() {
				return filepath.SkipDir
			}
			return nil
		}

		if d.IsDir() {
			return nil
		}
		if d.Type() == fs.ModeSymlink {
			return nil
		}

		info, err := d.Info()
		if err != nil {
			fmt.Fprintf(os.Stderr, "scan#Scan#WalkDir file Info error: %v\n", err)
			return nil
		}

		entries = append(entries, Entry{
			RelPath: rel,
			Size:    info.Size(),
			ModTime: info.ModTime(),
		})

		return nil
	}); err != nil {
		return nil, err
	}
	return entries, nil
}
