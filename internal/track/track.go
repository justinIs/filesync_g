// Package track manages the state of files to understand when they have changed
package track

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"time"

	"github.com/justinIs/filesync_g/internal/config"
	"github.com/justinIs/filesync_g/internal/scan"
)

type ManifestFileInfo struct {
	RelPath string
	Size    int64
	ModTime time.Time
	Hash    string
}

type Manifest struct {
	source string
	// manifest for files keyed by relative file path
	files map[string]ManifestFileInfo
}

type manifestFile struct {
	Version int          `json:"version"`
	Files   []fileRecord `json:"files"`
}

type fileRecord struct {
	RelPath string    `json:"relPath"`
	Size    int64     `json:"size"`
	ModTime time.Time `json:"modTime"`
	Hash    string    `json:"hash"`
}

var ErrInvalidManifest = errors.New("invalid manifest file")

const manifestVersion = 1

func LoadManifest(source string) (*Manifest, error) {
	m := &Manifest{
		source: source,
		files:  make(map[string]ManifestFileInfo),
	}

	path := filepath.Join(source, config.ManifestFile)
	data, err := os.ReadFile(path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return m, nil
		}
		return nil, fmt.Errorf("manifest load error (%s): %w", path, err)
	}

	var mf manifestFile
	if err := json.Unmarshal(data, &mf); err != nil {
		return nil, fmt.Errorf("%w (%s): %w", ErrInvalidManifest, path, err)
	}
	if mf.Version != manifestVersion {
		return nil, fmt.Errorf("%w (%s): unsupported version %d", ErrInvalidManifest, path, mf.Version)
	}

	for _, r := range mf.Files {
		m.files[r.RelPath] = ManifestFileInfo(r)
	}

	return m, nil
}

type CheckEntriesResult struct {
	Updates        []ManifestFileInfo
	Deletes        []ManifestFileInfo
	Refreshes      []ManifestFileInfo
	ExistingCount  int
	UntouchedCount int
}

func (m *Manifest) CheckEntries(ctx context.Context, entries []scan.Entry) (CheckEntriesResult, error) {
	result := CheckEntriesResult{
		ExistingCount:  len(m.files),
		UntouchedCount: 0,
	}

	seen := make(map[string]struct{}, len(entries))

	for _, e := range entries {

		fi, ok := m.files[e.RelPath]

		seen[e.RelPath] = struct{}{}

		if ok && fi.Size == e.Size && fi.ModTime.Equal(e.ModTime) {
			// files match
			result.UntouchedCount++
			continue
		}

		hash, skip, err := m.hashEntry(ctx, e.RelPath)
		if err != nil {
			return CheckEntriesResult{}, err
		}

		if skip {
			// skip due to non-critical error from hashEntry
			result.UntouchedCount++
			continue
		}

		if ok && hash == fi.Hash {
			result.Refreshes = append(result.Refreshes, ManifestFileInfo{
				RelPath: e.RelPath,
				Size:    e.Size,
				ModTime: e.ModTime,
				Hash:    hash,
			})
			continue
		}

		result.Updates = append(result.Updates, ManifestFileInfo{
			RelPath: e.RelPath,
			Size:    e.Size,
			ModTime: e.ModTime,
			Hash:    hash,
		})
	}

	for relPath, fi := range m.files {
		if _, ok := seen[relPath]; !ok {
			result.Deletes = append(result.Deletes, fi)
		}
	}
	sort.Slice(result.Deletes, func(i, j int) bool {
		return result.Deletes[i].RelPath < result.Deletes[j].RelPath
	})

	return result, nil
}

// AddFile adds a single file to the manifest and saves to disk
func (m *Manifest) AddFile(ctx context.Context, relPath string) error {
	info, err := os.Stat(filepath.Join(m.source, relPath))
	if err != nil {
		return err
	}
	hash, err := m.generateHash(ctx, relPath)
	if err != nil {
		return err
	}
	fi := ManifestFileInfo{
		RelPath: relPath,
		Size:    info.Size(),
		ModTime: info.ModTime(),
		Hash:    hash,
	}
	m.files[fi.RelPath] = fi
	return m.save()
}

func (m *Manifest) applySyncOutcomes(ocs ...SyncOutcome) {
	for _, oc := range ocs {
		switch oc.Op {
		case OpKindUpdate, OpKindRefresh:
			m.files[oc.Info.RelPath] = oc.Info
		case OpKindDelete:
			delete(m.files, oc.Info.RelPath)
		}
	}
}

func (m *Manifest) save() (err error) {
	records := make([]fileRecord, 0, len(m.files))
	for _, fi := range m.files {
		records = append(records, fileRecord(fi))
	}
	sort.Slice(records, func(i, j int) bool {
		return records[i].RelPath < records[j].RelPath
	})

	data, err := json.MarshalIndent(manifestFile{Version: manifestVersion, Files: records}, "", "  ")
	if err != nil {
		return fmt.Errorf("save: error marshalling manifest: %w", err)
	}

	dir := filepath.Join(m.source, config.ManifestDir)
	if err = os.MkdirAll(dir, 0o755); err != nil {
		return fmt.Errorf("save: create %s: %w", dir, err)
	}

	tmp, err := os.CreateTemp(dir, "state-*.tmp")
	if err != nil {
		return fmt.Errorf("save: create temp manifest: %w", err)
	}
	defer func() {
		if err != nil {
			_ = tmp.Close()
			_ = os.Remove(tmp.Name())
		}
	}()

	if _, err := tmp.Write(data); err != nil {
		return fmt.Errorf("save: write temp manifest: %w", err)
	}
	if err := tmp.Sync(); err != nil {
		return fmt.Errorf("save: sync temp manifest: %w", err)
	}

	if err := tmp.Close(); err != nil {
		return fmt.Errorf("save: close temp manifest: %w", err)
	}

	path := filepath.Join(m.source, config.ManifestFile)
	if err := os.Rename(tmp.Name(), path); err != nil {
		return fmt.Errorf("save: replace %s: %w", path, err)
	}

	return nil
}

type ctxReader struct {
	ctx context.Context
	r   io.Reader
}

func (cr *ctxReader) Read(p []byte) (int, error) {
	if err := cr.ctx.Err(); err != nil {
		return 0, err
	}
	return cr.r.Read(p)
}

func (m *Manifest) generateHash(ctx context.Context, relPath string) (string, error) {
	path := filepath.Join(m.source, relPath)

	f, err := os.Open(path)
	if err != nil {
		return "", err
	}
	defer func() {
		_ = f.Close()
	}()

	h := sha256.New()
	if _, err := io.Copy(h, &ctxReader{ctx: ctx, r: f}); err != nil {
		return "", fmt.Errorf("generateHash %s: %w", path, err)
	}

	return hex.EncodeToString(h.Sum(nil)), nil
}

// hashEntry hashes the file and classifies failures: a missing or unreadable
// file is a skip (warn and continue), anything else is fatal
func (m *Manifest) hashEntry(ctx context.Context, relPath string) (hash string, skip bool, err error) {
	hash, err = m.generateHash(ctx, relPath)
	switch {
	case errors.Is(err, os.ErrNotExist), errors.Is(err, os.ErrPermission):
		fmt.Fprintf(os.Stderr, "track: skipping %s: %v\n", relPath, err)
		return "", true, nil
	case err != nil:
		return "", false, err
	default:
		return hash, false, nil
	}
}
