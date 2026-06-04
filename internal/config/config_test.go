package config

import (
	"os"
	"path/filepath"
	"testing"
)

// writeConfig writes content to a temp .toml file and returns its path.
func writeConfig(t *testing.T, content string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "filesync.toml")
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("write config: %v", err)
	}
	return path
}

// Load reads a TOML file, decodes the ignore list, and compiles the matcher.
// This exercises the whole pipeline: decode -> field mapping -> compile.
func TestLoad_ParsesIgnoreAndMatches(t *testing.T) {
	path := writeConfig(t, `
ignore = ["*.tmp", "node_modules/", "build/cache"]
`)

	cfg, err := Load(path)
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
	path := writeConfig(t, "")

	cfg, err := Load(path)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if cfg.ShouldIgnore("anything", false) {
		t.Error("config with no ignore list should ignore nothing")
	}
}

func TestLoad_MissingFileReturnsError(t *testing.T) {
	missing := filepath.Join(t.TempDir(), "does-not-exist.toml")

	if _, err := Load(missing); err == nil {
		t.Fatal("expected an error loading a missing config, got nil")
	}
}

func TestLoad_InvalidIgnorePatternReturnsError(t *testing.T) {
	path := writeConfig(t, `ignore = ["["]`) // unclosed character class

	if _, err := Load(path); err == nil {
		t.Fatal("expected an error for an invalid ignore pattern, got nil")
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
