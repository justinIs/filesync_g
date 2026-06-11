package scan

import (
	"os"
	"path/filepath"
	"sort"
	"testing"
)

// fakeIgnorer is a test stub for the Ignorer interface. Because Scan depends on
// the interface (not on config), tests inject their own ignore decisions here
// without constructing a real config.Config.
type fakeIgnorer struct {
	ignore map[string]bool // relPath (forward slashes) -> should ignore?
}

func (f fakeIgnorer) ShouldIgnore(relPath string, isDir bool) bool {
	return f.ignore[relPath]
}

// writeFile creates root/rel (with parent dirs) containing content.
func writeFile(t *testing.T, root, rel, content string) {
	t.Helper()
	full := filepath.Join(root, filepath.FromSlash(rel))
	if err := os.MkdirAll(filepath.Dir(full), 0o755); err != nil {
		t.Fatalf("mkdir for %s: %v", rel, err)
	}
	if err := os.WriteFile(full, []byte(content), 0o644); err != nil {
		t.Fatalf("write %s: %v", rel, err)
	}
}

// relPaths returns the RelPath of every entry, sorted, so comparisons are
// order-independent and failure messages are stable.
func relPaths(entries []Entry) []string {
	got := make([]string, len(entries))
	for i, e := range entries {
		got[i] = e.RelPath
	}
	sort.Strings(got)
	return got
}

func equalStrings(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

// buildTree lays down a standard fixture used by most tests:
//
//	a.txt              keep
//	sub/b.txt          keep
//	sub/c.tmp          ignored (file)
//	node_modules/x.txt ignored (whole dir pruned)
func buildTree(t *testing.T) string {
	t.Helper()
	root := t.TempDir()
	writeFile(t, root, "a.txt", "hello")
	writeFile(t, root, "sub/b.txt", "world!")
	writeFile(t, root, "sub/c.tmp", "temp data")
	writeFile(t, root, "node_modules/x.txt", "dependency")
	return root
}

func standardIgnorer() fakeIgnorer {
	return fakeIgnorer{ignore: map[string]bool{
		"sub/c.tmp":    true, // ignore a single file
		"node_modules": true, // ignore a whole directory
	}}
}

func TestScan_CollectsKeptFilesWithForwardSlashes(t *testing.T) {
	root := buildTree(t)

	entries, err := Scan(t.Context(), root, standardIgnorer())
	if err != nil {
		t.Fatalf("Scan: %v", err)
	}

	want := []string{"a.txt", "sub/b.txt"}
	got := relPaths(entries)
	if !equalStrings(got, want) {
		t.Errorf("RelPaths = %v, want %v", got, want)
	}
}

func TestScan_IgnoresFileButKeepsSiblings(t *testing.T) {
	root := buildTree(t)

	entries, err := Scan(t.Context(), root, standardIgnorer())
	if err != nil {
		t.Fatalf("Scan: %v", err)
	}

	got := relPaths(entries)
	for _, p := range got {
		if p == "sub/c.tmp" {
			t.Errorf("ignored file sub/c.tmp was included: %v", got)
		}
	}
	// The sibling in the same directory must survive.
	found := false
	for _, p := range got {
		if p == "sub/b.txt" {
			found = true
		}
	}
	if !found {
		t.Errorf("sibling sub/b.txt missing after ignoring sub/c.tmp: %v", got)
	}
}

func TestScan_PrunesIgnoredDirectorySubtree(t *testing.T) {
	root := buildTree(t)

	entries, err := Scan(t.Context(), root, standardIgnorer())
	if err != nil {
		t.Fatalf("Scan: %v", err)
	}

	for _, e := range entries {
		if e.RelPath == "node_modules" || filepath.ToSlash(e.RelPath) == "node_modules/x.txt" {
			t.Errorf("entry under ignored dir was included: %q", e.RelPath)
		}
	}
}

func TestScan_DoesNotRecordDirectories(t *testing.T) {
	root := buildTree(t)

	entries, err := Scan(t.Context(), root, standardIgnorer())
	if err != nil {
		t.Fatalf("Scan: %v", err)
	}

	// "sub" is a kept directory; it must not appear as an entry itself.
	for _, e := range entries {
		if e.RelPath == "sub" {
			t.Errorf("directory %q was recorded as an entry", e.RelPath)
		}
	}
}

func TestScan_RootNotRecorded(t *testing.T) {
	root := buildTree(t)

	entries, err := Scan(t.Context(), root, standardIgnorer())
	if err != nil {
		t.Fatalf("Scan: %v", err)
	}

	for _, e := range entries {
		if e.RelPath == "." || e.RelPath == "" {
			t.Errorf("root recorded as entry: %q", e.RelPath)
		}
	}
}

func TestScan_ReportsFileSize(t *testing.T) {
	root := t.TempDir()
	const content = "hello" // 5 bytes
	writeFile(t, root, "a.txt", content)

	entries, err := Scan(t.Context(), root, fakeIgnorer{})
	if err != nil {
		t.Fatalf("Scan: %v", err)
	}

	var found bool
	for _, e := range entries {
		if e.RelPath == "a.txt" {
			found = true
			if e.Size != int64(len(content)) {
				t.Errorf("Size = %d, want %d", e.Size, len(content))
			}
			if e.ModTime.IsZero() {
				t.Error("ModTime is zero, want a real timestamp")
			}
		}
	}
	if !found {
		t.Fatalf("a.txt not found in entries")
	}
}

func TestScan_EmptyDirectoryReturnsNoEntries(t *testing.T) {
	root := t.TempDir()

	entries, err := Scan(t.Context(), root, fakeIgnorer{})
	if err != nil {
		t.Fatalf("Scan: %v", err)
	}
	if len(entries) != 0 {
		t.Errorf("expected no entries for empty dir, got %v", relPaths(entries))
	}
}

func TestScan_NonexistentRootReturnsError(t *testing.T) {
	missing := filepath.Join(t.TempDir(), "does-not-exist")

	_, err := Scan(t.Context(), missing, fakeIgnorer{})
	if err == nil {
		t.Fatal("expected an error scanning a nonexistent root, got nil")
	}
}
