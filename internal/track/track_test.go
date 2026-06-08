package track

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/justinIs/filesync_g/internal/config"
	"github.com/justinIs/filesync_g/internal/scan"
)

// writeManifest writes raw bytes to the manifest path under source, creating
// the manifest directory. Used to stage missing/corrupt/version-mismatch states.
func writeManifest(t *testing.T, source string, data []byte) {
	t.Helper()
	if err := os.MkdirAll(filepath.Join(source, config.ManifestDir), 0o755); err != nil {
		t.Fatalf("mkdir manifest dir: %v", err)
	}
	if err := os.WriteFile(filepath.Join(source, config.ManifestFile), data, 0o644); err != nil {
		t.Fatalf("write manifest: %v", err)
	}
}

// TestSerializationRoundTrip seeds state directly with a known record (fixed
// mtime, so it's deterministic and exercises nanosecond precision) and asserts
// it survives save -> load unchanged. This isolates serialization from AddFile.
func TestSerializationRoundTrip(t *testing.T) {
	source := t.TempDir()

	want := ManifestFileInfo{
		RelPath: "dir/test.txt",
		Size:    12,
		ModTime: time.Date(2026, 6, 8, 14, 30, 0, 123456789, time.UTC),
		Hash:    "0123456789abcdef",
	}

	m := &Manifest{
		source: source,
		files:  map[string]ManifestFileInfo{want.RelPath: want},
	}
	if err := m.save(); err != nil {
		t.Fatalf("save: %v", err)
	}

	reloaded, err := LoadManifest(source)
	if err != nil {
		t.Fatalf("load: %v", err)
	}

	if len(reloaded.files) != 1 {
		t.Fatalf("want 1 file, got %d: %+v", len(reloaded.files), reloaded.files)
	}
	got, ok := reloaded.files[want.RelPath]
	if !ok {
		t.Fatalf("file %q missing after reload: %+v", want.RelPath, reloaded.files)
	}

	if got.RelPath != want.RelPath || got.Size != want.Size || got.Hash != want.Hash {
		t.Errorf("reloaded mismatch:\n want %+v\n got  %+v", want, got)
	}
	// time.Time must be compared with Equal, not ==: the JSON round-trip returns
	// a time with the same instant but a different *Location, so == would fail.
	if !got.ModTime.Equal(want.ModTime) {
		t.Errorf("reloaded ModTime: want %v, got %v", want.ModTime, got.ModTime)
	}
}

// TestAddFile checks that AddFile populates the record from the real file,
// including the exact SHA-256 of the content.
func TestAddFile(t *testing.T) {
	source := t.TempDir()

	m, err := LoadManifest(source)
	if err != nil {
		t.Fatalf("load: %v", err)
	}

	const (
		name    = "test.txt"
		content = "test content"
	)
	if err := os.WriteFile(filepath.Join(source, name), []byte(content), 0o644); err != nil {
		t.Fatalf("write file: %v", err)
	}

	if err := m.AddFile(name); err != nil {
		t.Fatalf("add file: %v", err)
	}

	fi, ok := m.files[name]
	if !ok {
		t.Fatalf("file %q not tracked: %+v", name, m.files)
	}

	sum := sha256.Sum256([]byte(content))
	wantHash := hex.EncodeToString(sum[:])

	if fi.RelPath != name {
		t.Errorf("RelPath: want %q, got %q", name, fi.RelPath)
	}
	if fi.Size != int64(len(content)) {
		t.Errorf("Size: want %d, got %d", len(content), fi.Size)
	}
	if fi.Hash != wantHash {
		t.Errorf("Hash: want %s, got %s", wantHash, fi.Hash)
	}
	if fi.ModTime.IsZero() {
		t.Error("ModTime is zero")
	}
}

// TestLoadMissingManifest: a first run (no manifest file) is not an error,
// it yields an empty initialized manifest.
func TestLoadMissingManifest(t *testing.T) {
	source := t.TempDir()

	m, err := LoadManifest(source)
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	if m.files == nil {
		t.Fatal("files map is nil; expected initialized empty map")
	}
	if len(m.files) != 0 {
		t.Fatalf("want empty manifest, got %d files: %+v", len(m.files), m.files)
	}
}

// TestLoadCorruptManifest: an unparseable manifest must surface as
// ErrInvalidManifest, never be silently treated as empty (which would
// re-upload the whole tree).
func TestLoadCorruptManifest(t *testing.T) {
	source := t.TempDir()
	writeManifest(t, source, []byte("{not valid json"))

	_, err := LoadManifest(source)
	if !errors.Is(err, ErrInvalidManifest) {
		t.Fatalf("want ErrInvalidManifest, got %v", err)
	}
}

// TestLoadWrongVersion: a manifest with an unsupported schema version must
// surface as ErrInvalidManifest rather than load partial/wrong state.
func TestLoadWrongVersion(t *testing.T) {
	source := t.TempDir()
	writeManifest(t, source, []byte(`{"version": 999, "files": []}`))

	_, err := LoadManifest(source)
	if !errors.Is(err, ErrInvalidManifest) {
		t.Fatalf("want ErrInvalidManifest, got %v", err)
	}
}

// --- CheckEntries fixtures ---

func sha256Hex(content string) string {
	sum := sha256.Sum256([]byte(content))
	return hex.EncodeToString(sum[:])
}

// writeFileAt writes content at relPath under source with a fixed mtime, then
// returns the scan.Entry as the scanner would see it (size and mtime read back
// from stat, so it reflects whatever the filesystem actually stored).
func writeFileAt(t *testing.T, source, relPath, content string, mtime time.Time) scan.Entry {
	t.Helper()
	p := filepath.Join(source, relPath)
	if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	if err := os.WriteFile(p, []byte(content), 0o644); err != nil {
		t.Fatalf("write %s: %v", relPath, err)
	}
	if err := os.Chtimes(p, mtime, mtime); err != nil {
		t.Fatalf("chtimes %s: %v", relPath, err)
	}
	info, err := os.Stat(p)
	if err != nil {
		t.Fatalf("stat %s: %v", relPath, err)
	}
	return scan.Entry{RelPath: relPath, Size: info.Size(), ModTime: info.ModTime()}
}

func manifestWith(source string, files ...ManifestFileInfo) *Manifest {
	m := &Manifest{source: source, files: make(map[string]ManifestFileInfo)}
	for _, f := range files {
		m.files[f.RelPath] = f
	}
	return m
}

func infoByPath(s []ManifestFileInfo, relPath string) (ManifestFileInfo, bool) {
	for _, fi := range s {
		if fi.RelPath == relPath {
			return fi, true
		}
	}
	return ManifestFileInfo{}, false
}

var (
	timeOld = time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	timeNew = time.Date(2026, 6, 1, 12, 0, 0, 0, time.UTC)
)

// TestCheckEntriesNewFile: a file absent from state is an Update carrying its
// freshly computed hash.
func TestCheckEntriesNewFile(t *testing.T) {
	source := t.TempDir()
	e := writeFileAt(t, source, "a.txt", "hello", timeNew)
	m := manifestWith(source) // empty state

	res, err := m.CheckEntries([]scan.Entry{e})
	if err != nil {
		t.Fatalf("CheckEntries: %v", err)
	}

	if len(res.Refreshes) != 0 || len(res.Deletes) != 0 {
		t.Fatalf("want only an Update, got %+v", res)
	}
	up, ok := infoByPath(res.Updates, "a.txt")
	if !ok {
		t.Fatalf("a.txt not in Updates: %+v", res.Updates)
	}
	if up.Hash != sha256Hex("hello") {
		t.Errorf("Hash: want %s, got %s", sha256Hex("hello"), up.Hash)
	}
	if up.Size != e.Size || !up.ModTime.Equal(e.ModTime) {
		t.Errorf("metadata mismatch: want size=%d mtime=%v, got %+v", e.Size, e.ModTime, up)
	}
}

// TestCheckEntriesUnchanged: a record matching the entry's size and mtime yields
// no change in any bucket. The assertion confirms the file is treated as
// unchanged; it cannot directly observe that hashing was skipped (the in-memory
// seed shares the entry's mtime Location, so == would pass here too — the
// Equal-vs-== guard lives in TestCheckEntriesUnchangedAfterReload).
func TestCheckEntriesUnchanged(t *testing.T) {
	source := t.TempDir()
	e := writeFileAt(t, source, "a.txt", "hello", timeNew)
	m := manifestWith(source, ManifestFileInfo{
		RelPath: "a.txt", Size: e.Size, ModTime: e.ModTime, Hash: sha256Hex("hello"),
	})

	res, err := m.CheckEntries([]scan.Entry{e})
	if err != nil {
		t.Fatalf("CheckEntries: %v", err)
	}
	if len(res.Updates) != 0 || len(res.Refreshes) != 0 || len(res.Deletes) != 0 {
		t.Fatalf("want empty result, got %+v", res)
	}
}

// TestCheckEntriesUnchangedDifferentZone guards the Equal-vs-== comparison in the
// detection fast path. A scan stat carries time.Local; a stored mtime can carry a
// different Location representing the same instant — this happens when a JSON
// reload lands across a DST boundary or timezone change, so the saved offset no
// longer matches Local and time.Parse returns a fixed zone instead of Local.
// Under == that compares unequal and the file re-hashes every run until touched;
// only ModTime.Equal treats them as matching. Seeding the state mtime in UTC
// forces the Location mismatch deterministically, independent of the test host's
// timezone (a plain same-host round-trip would not, since Parse restores Local).
func TestCheckEntriesUnchangedDifferentZone(t *testing.T) {
	source := t.TempDir()
	e := writeFileAt(t, source, "a.txt", "hello", timeNew)

	// Same instant as the entry's stat mtime, but a different Location.
	stateMtime := e.ModTime.UTC()
	if stateMtime.Location() == e.ModTime.Location() {
		t.Skip("test host is UTC; cannot force a Location mismatch")
	}
	m := manifestWith(source, ManifestFileInfo{
		RelPath: "a.txt", Size: e.Size, ModTime: stateMtime, Hash: sha256Hex("hello"),
	})

	res, err := m.CheckEntries([]scan.Entry{e})
	if err != nil {
		t.Fatalf("CheckEntries: %v", err)
	}
	if len(res.Updates) != 0 || len(res.Refreshes) != 0 || len(res.Deletes) != 0 {
		t.Fatalf("same-instant different-zone mtime should be unchanged, got %+v", res)
	}
}

// TestCheckEntriesChangedContent: differing metadata plus a differing hash is an
// Update with the new hash.
func TestCheckEntriesChangedContent(t *testing.T) {
	source := t.TempDir()
	e := writeFileAt(t, source, "a.txt", "new content", timeNew)
	m := manifestWith(source, ManifestFileInfo{
		RelPath: "a.txt", Size: 3, ModTime: timeOld, Hash: sha256Hex("old"),
	})

	res, err := m.CheckEntries([]scan.Entry{e})
	if err != nil {
		t.Fatalf("CheckEntries: %v", err)
	}
	if len(res.Refreshes) != 0 || len(res.Deletes) != 0 {
		t.Fatalf("want only an Update, got %+v", res)
	}
	up, ok := infoByPath(res.Updates, "a.txt")
	if !ok {
		t.Fatalf("a.txt not in Updates: %+v", res.Updates)
	}
	if up.Hash != sha256Hex("new content") {
		t.Errorf("Hash: want %s, got %s", sha256Hex("new content"), up.Hash)
	}
}

// TestCheckEntriesTouchOnly: mtime moved but content is identical (hash matches
// state) — a Refresh, never an Update. Guards against re-uploading unchanged data.
func TestCheckEntriesTouchOnly(t *testing.T) {
	source := t.TempDir()
	const content = "stable"
	e := writeFileAt(t, source, "a.txt", content, timeNew)
	m := manifestWith(source, ManifestFileInfo{
		RelPath: "a.txt", Size: e.Size, ModTime: timeOld, Hash: sha256Hex(content),
	})

	res, err := m.CheckEntries([]scan.Entry{e})
	if err != nil {
		t.Fatalf("CheckEntries: %v", err)
	}
	if len(res.Updates) != 0 || len(res.Deletes) != 0 {
		t.Fatalf("want only a Refresh, got %+v", res)
	}
	rf, ok := infoByPath(res.Refreshes, "a.txt")
	if !ok {
		t.Fatalf("a.txt not in Refreshes: %+v", res.Refreshes)
	}
	// The refresh should carry the current mtime so state stops re-hashing it.
	if !rf.ModTime.Equal(e.ModTime) {
		t.Errorf("refresh ModTime: want %v, got %v", e.ModTime, rf.ModTime)
	}
}

// TestCheckEntriesDeleted: a record in state with no corresponding entry is a
// Delete; a still-present file is not, even when listed alongside it.
func TestCheckEntriesDeleted(t *testing.T) {
	source := t.TempDir()
	present := writeFileAt(t, source, "present.txt", "hi", timeNew)
	m := manifestWith(
		source,
		ManifestFileInfo{RelPath: present.RelPath, Size: present.Size, ModTime: present.ModTime, Hash: sha256Hex("hi")},
		ManifestFileInfo{RelPath: "gone.txt", Size: 5, ModTime: timeOld, Hash: "deadbeef"},
	)

	res, err := m.CheckEntries([]scan.Entry{present})
	if err != nil {
		t.Fatalf("CheckEntries: %v", err)
	}
	if _, ok := infoByPath(res.Deletes, "gone.txt"); !ok {
		t.Errorf("gone.txt not in Deletes: %+v", res.Deletes)
	}
	if _, ok := infoByPath(res.Deletes, "present.txt"); ok {
		t.Errorf("present.txt wrongly marked deleted: %+v", res.Deletes)
	}
	if len(res.Updates) != 0 || len(res.Refreshes) != 0 {
		t.Errorf("present.txt should be unchanged, got %+v", res)
	}
}

// TestCheckEntriesSkipMissing: a file the scan listed but that is gone at hash
// time is skipped — no bucket, no Delete (it's still "seen"), no error.
func TestCheckEntriesSkipMissing(t *testing.T) {
	source := t.TempDir()
	ghost := scan.Entry{RelPath: "ghost.txt", Size: 10, ModTime: timeNew} // never written
	m := manifestWith(source)

	res, err := m.CheckEntries([]scan.Entry{ghost})
	if err != nil {
		t.Fatalf("CheckEntries: %v", err)
	}
	if len(res.Updates) != 0 || len(res.Refreshes) != 0 || len(res.Deletes) != 0 {
		t.Fatalf("want empty result for skipped file, got %+v", res)
	}
}

// TestCheckEntriesEmptyScanDeletesAll documents current behavior: an empty scan
// against populated state marks every file for deletion. This is correct for the
// detection layer, but it's the dangerous input (wrong source dir, failed mount)
// that any empty-scan/delete-propagation guard must defend against. When such a
// guard is added, this test's expectation should change with it.
func TestCheckEntriesEmptyScanDeletesAll(t *testing.T) {
	source := t.TempDir()
	m := manifestWith(
		source,
		ManifestFileInfo{RelPath: "a.txt", Size: 1, ModTime: timeOld, Hash: "a"},
		ManifestFileInfo{RelPath: "b.txt", Size: 2, ModTime: timeOld, Hash: "b"},
	)

	res, err := m.CheckEntries(nil)
	if err != nil {
		t.Fatalf("CheckEntries: %v", err)
	}
	if len(res.Updates) != 0 || len(res.Refreshes) != 0 {
		t.Fatalf("want only Deletes, got %+v", res)
	}
	if len(res.Deletes) != 2 {
		t.Fatalf("want 2 deletes, got %d: %+v", len(res.Deletes), res.Deletes)
	}
	for _, rel := range []string{"a.txt", "b.txt"} {
		if _, ok := infoByPath(res.Deletes, rel); !ok {
			t.Errorf("%s not in Deletes: %+v", rel, res.Deletes)
		}
	}
}
