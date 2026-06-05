package config

import (
	"errors"
	"os"
	"path/filepath"
	"testing"
)

// writeConfig writes content to a temp .toml file and returns the source directory
func writeConfig(t *testing.T, content string) string {
	t.Helper()
	source := t.TempDir()
	path := filepath.Join(source, "filesync.toml")
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("write config: %v", err)
	}
	return source
}

// Load reads a TOML file, decodes the ignore list, and compiles the matcher.
// This exercises the whole pipeline: decode -> field mapping -> compile.
func TestLoad_ParsesIgnoreAndMatches(t *testing.T) {
	source := writeConfig(t, `
ignore = ["*.tmp", "node_modules/", "build/cache"]
`)

	cfg, err := Load(source)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}

	tests := []struct {
		name  string
		path  string
		isDir bool
		want  bool
	}{
		{name: "ignored glob", path: "src/a.tmp", isDir: false, want: true},
		{name: "ignored dir", path: "node_modules", isDir: true, want: true},
		{name: "ignored dir as file", path: "node_modules", isDir: false, want: false},
		{name: "ignored anchored", path: "build/cache", isDir: true, want: true},
		{name: "kept file", path: "src/main.go", isDir: false, want: false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := cfg.ShouldIgnore(tt.path, tt.isDir); got != tt.want {
				t.Errorf("ShouldIgnore(%q, isDir=%v) = %v, want %v",
					tt.path, tt.isDir, got, tt.want)
			}
		})
	}
}

func TestLoad_EmptyIgnoreList(t *testing.T) {
	source := writeConfig(t, "")

	cfg, err := Load(source)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if cfg.ShouldIgnore("anything", false) {
		t.Error("config with no user ignore list should ignore non-built-in paths")
	}
}

// Load always applies built-in ignores for the config file and the .filesync
// metadata dir, regardless of (and in addition to) the user's ignore list.
// The .filesync entry is directory-only: the scanner prunes the whole subtree
// via SkipDir, so ShouldIgnore matches the directory itself, not files within.
func TestLoad_BuiltinIgnores(t *testing.T) {
	source := writeConfig(t, "") // empty user ignore list — only built-ins apply

	cfg, err := Load(source)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}

	tests := []struct {
		name  string
		path  string
		isDir bool
		want  bool
	}{
		{name: "config file at root", path: ConfigFile, isDir: false, want: true},
		{name: "config file nested (unanchored)", path: "sub/" + ConfigFile, isDir: false, want: true},
		{name: "manifest dir", path: ManifestDir, isDir: true, want: true},
		{name: "manifest dir as a file is not skipped", path: ManifestDir, isDir: false, want: false},
		{name: "unrelated file kept", path: "src/main.go", isDir: false, want: false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := cfg.ShouldIgnore(tt.path, tt.isDir); got != tt.want {
				t.Errorf("ShouldIgnore(%q, isDir=%v) = %v, want %v",
					tt.path, tt.isDir, got, tt.want)
			}
		})
	}
}

func TestLoad_MissingFileReturnsError(t *testing.T) {
	source := filepath.Join(t.TempDir(), "does-not-exist")

	if _, err := Load(source); !errors.Is(err, ErrMissingConfig) {
		t.Fatalf("expected ErrMissingConfig, got %v", err)
	}
}

func TestLoad_InvalidTOMLReturnsError(t *testing.T) {
	source := writeConfig(t, `ignore = [`) // unterminated array

	if _, err := Load(source); !errors.Is(err, ErrInvalidConfig) {
		t.Fatalf("expected ErrInvalidConfig, got %v", err)
	}
}

func TestLoad_InvalidIgnorePatternReturnsError(t *testing.T) {
	source := writeConfig(t, `ignore = ["["]`) // unclosed character class

	if _, err := Load(source); !errors.Is(err, ErrInvalidIgnorePattern) {
		t.Fatalf("expected ErrInvalidIgnorePattern, got %v", err)
	}
}

// A zero-value Config (not built via Load) has no matcher; ShouldIgnore must
// not panic and must ignore nothing.
func TestShouldIgnore_NilMatcher(t *testing.T) {
	var c Config
	if c.ShouldIgnore("anything", false) {
		t.Error("zero-value Config should ignore nothing")
	}
}
