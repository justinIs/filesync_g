package track

import (
	"errors"
	"fmt"
	"sync"
	"testing"
)

// --- Committer fixtures ---

func updOutcome(relPath, hash string) SyncOutcome {
	return SyncOutcome{
		Op:   OpKindUpdate,
		Info: ManifestFileInfo{RelPath: relPath, Size: int64(len(hash)), ModTime: timeNew, Hash: hash},
	}
}

func refreshOutcome(relPath, hash string) SyncOutcome {
	return SyncOutcome{
		Op:   OpKindRefresh,
		Info: ManifestFileInfo{RelPath: relPath, ModTime: timeNew, Hash: hash},
	}
}

func delOutcome(relPath string) SyncOutcome {
	return SyncOutcome{Op: OpKindDelete, Info: ManifestFileInfo{RelPath: relPath}}
}

// TestCommitterAppliesOutcomes: a successful run applies updates and refreshes
// (overwrite) and deletes (remove), persists them, and leaves untouched records
// alone. Asserting against a fresh reload proves it went to disk, not just memory.
func TestCommitterAppliesOutcomes(t *testing.T) {
	source := t.TempDir()
	m := manifestWith(
		source,
		ManifestFileInfo{RelPath: "upd.txt", Size: 3, ModTime: timeOld, Hash: "old"},
		ManifestFileInfo{RelPath: "del.txt", Size: 1, ModTime: timeOld, Hash: "d"},
		ManifestFileInfo{RelPath: "refresh.txt", Size: 5, ModTime: timeOld, Hash: "samehash"},
		ManifestFileInfo{RelPath: "keep.txt", Size: 2, ModTime: timeOld, Hash: "k"},
	)

	c := NewCommitter(m)
	c.Send(updOutcome("upd.txt", "newhash"))
	c.Send(updOutcome("new.txt", "freshhash"))
	c.Send(refreshOutcome("refresh.txt", "samehash"))
	c.Send(delOutcome("del.txt"))
	if err := c.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}

	reloaded, err := LoadManifest(source)
	if err != nil {
		t.Fatalf("reload: %v", err)
	}

	if fi, ok := reloaded.files["upd.txt"]; !ok || fi.Hash != "newhash" {
		t.Errorf("upd.txt: want hash newhash, got %+v (ok=%v)", fi, ok)
	}
	if fi, ok := reloaded.files["new.txt"]; !ok || fi.Hash != "freshhash" {
		t.Errorf("new.txt: want hash freshhash, got %+v (ok=%v)", fi, ok)
	}
	if fi, ok := reloaded.files["refresh.txt"]; !ok || !fi.ModTime.Equal(timeNew) {
		t.Errorf("refresh.txt: want mtime %v, got %+v (ok=%v)", timeNew, fi, ok)
	}
	if _, ok := reloaded.files["del.txt"]; ok {
		t.Errorf("del.txt should have been deleted: %+v", reloaded.files)
	}
	if fi, ok := reloaded.files["keep.txt"]; !ok || !fi.ModTime.Equal(timeOld) {
		t.Errorf("keep.txt should be untouched: %+v", reloaded.files)
	}
}

// TestCommitterSkipsFailedOutcome: an outcome carrying an error must never be
// applied — the manifest only ever advances for work that actually succeeded.
func TestCommitterSkipsFailedOutcome(t *testing.T) {
	source := t.TempDir()
	m := manifestWith(source)

	c := NewCommitter(m)
	bad := updOutcome("bad.txt", "x")
	bad.Err = errors.New("upload failed")
	c.Send(bad)
	c.Send(updOutcome("good.txt", "y"))
	if err := c.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}

	reloaded, err := LoadManifest(source)
	if err != nil {
		t.Fatalf("reload: %v", err)
	}
	if _, ok := reloaded.files["bad.txt"]; ok {
		t.Errorf("failed outcome must not be applied: %+v", reloaded.files)
	}
	if _, ok := reloaded.files["good.txt"]; !ok {
		t.Errorf("good.txt should be applied: %+v", reloaded.files)
	}
}

// TestCommitterPersistsAllOnClose: sending more outcomes than the channel buffer
// and then closing must persist every one — proves the drain loop and the final
// coalesced batch are saved when the channel closes (no lost tail).
func TestCommitterPersistsAllOnClose(t *testing.T) {
	source := t.TempDir()
	m := manifestWith(source)

	c := NewCommitter(m)
	const n = 1000 // > channel buffer (500), so sends block and coalesce
	for i := 0; i < n; i++ {
		c.Send(updOutcome(fmt.Sprintf("f%04d.txt", i), fmt.Sprintf("h%d", i)))
	}
	if err := c.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}

	reloaded, err := LoadManifest(source)
	if err != nil {
		t.Fatalf("reload: %v", err)
	}
	if len(reloaded.files) != n {
		t.Fatalf("want %d files persisted, got %d", n, len(reloaded.files))
	}
}

// TestCommitterConcurrentSends drives the committer from many goroutines at once.
// The single-owner design means no locks on the manifest; run under -race to
// confirm there's no concurrent map access or save/mutate overlap.
func TestCommitterConcurrentSends(t *testing.T) {
	source := t.TempDir()
	m := manifestWith(source)

	c := NewCommitter(m)
	const goroutines, per = 8, 100
	var wg sync.WaitGroup
	for g := 0; g < goroutines; g++ {
		wg.Add(1)
		go func(g int) {
			defer wg.Done()
			for i := 0; i < per; i++ {
				c.Send(updOutcome(fmt.Sprintf("g%d-f%03d.txt", g, i), "h"))
			}
		}(g)
	}
	wg.Wait()
	if err := c.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}

	reloaded, err := LoadManifest(source)
	if err != nil {
		t.Fatalf("reload: %v", err)
	}
	if len(reloaded.files) != goroutines*per {
		t.Fatalf("want %d files, got %d", goroutines*per, len(reloaded.files))
	}
}
