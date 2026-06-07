// Package track manages the state of files to understand when they have changed
package track

import (
	"encoding/json"
	"errors"
	"fmt"
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

func (m *Manifest) Commit(fi ManifestFileInfo) error {
	m.files[fi.RelPath] = fi
	return m.save()
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

func (m *Manifest) generateHash(path string) string {
	return ""
}

type CheckEntriesResult struct {
	Updates []ManifestFileInfo
	Deletes []ManifestFileInfo
}

func (m *Manifest) CheckEntries(entries []scan.Entry) CheckEntriesResult {
	result := CheckEntriesResult{}
	// TODO: check for files to delete

	for _, e := range entries {
		manifest, ok := m.files[e.RelPath]

		if !ok {
			result.Updates = append(result.Updates, ManifestFileInfo{
				RelPath: e.RelPath,
				Size:    e.Size,
				ModTime: e.ModTime,
				Hash:    m.generateHash(e.RelPath),
			})
			continue
		}

		if manifest.Size == e.Size && e.ModTime.Equal(manifest.ModTime) {
			// files match
			continue
		}

		newHash := m.generateHash(e.RelPath)
		if newHash != manifest.Hash {
			result.Updates = append(result.Updates, ManifestFileInfo{
				RelPath: e.RelPath,
				Size:    e.Size,
				ModTime: e.ModTime,
				Hash:    newHash,
			})
		} else {
			// update metadata since hash is the same
		}
	}

	return result
}
