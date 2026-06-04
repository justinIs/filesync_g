package config

import (
	"errors"
	"path"
	"slices"
	"testing"
)

func Test_compile_RejectsInvalidIgnorePatterns(t *testing.T) {
	tests := []string{
		"[",
		"[a-",
		"[^abc",
		"a[",
		"\\",
	}

	for _, tt := range tests {
		t.Run(tt, func(t *testing.T) {
			_, err := compile([]string{tt})
			if !errors.Is(err, path.ErrBadPattern) {
				t.Errorf("got %v, want ErrBadPattern", err)
			}
		})
	}
}

func Test_compile_ValidCompileValues(t *testing.T) {
	tests := []struct {
		name     string
		input    []string
		expected []pattern
	}{{
		name:     "empty input",
		input:    []string{},
		expected: []pattern{},
	}, {
		name:     "blank/whitespace entries",
		input:    []string{"", " ", "\t"},
		expected: []pattern{},
	}, {
		name:  "1 empty, 1 valid",
		input: []string{"", "*.tmp"},
		expected: []pattern{{
			raw:      "*.tmp",
			glob:     "*.tmp",
			dirOnly:  false,
			anchored: false,
		}},
	}, {
		name:  "glob with whitespace",
		input: []string{"    *.tmp    "},
		expected: []pattern{{
			raw:      "    *.tmp    ",
			glob:     "*.tmp",
			dirOnly:  false,
			anchored: false,
		}},
	}, {
		name:  "directory",
		input: []string{"node_modules/"},
		expected: []pattern{{
			raw:      "node_modules/",
			glob:     "node_modules",
			dirOnly:  true,
			anchored: false,
		}},
	}, {
		name:  "bare name",
		input: []string{"*.tmp"},
		expected: []pattern{{
			raw:      "*.tmp",
			glob:     "*.tmp",
			dirOnly:  false,
			anchored: false,
		}},
	}, {
		name:  "anchored input",
		input: []string{"build/cache"},
		expected: []pattern{{
			raw:      "build/cache",
			glob:     "build/cache",
			dirOnly:  false,
			anchored: true,
		}},
	}}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			m, err := compile(tt.input)
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}

			if !slices.Equal(m.patterns, tt.expected) {
				t.Errorf("patterns = %+v, want %+v", m.patterns, tt.expected)
			}
		})
	}
}

func mustCompile(t *testing.T, patterns ...string) *ignoreMatcher {
	t.Helper()
	m, err := compile(patterns)
	if err != nil {
		t.Fatalf("compile(%v): %v", patterns, err)
	}
	return m
}

func Test_match_ValidIgnorePatterns(t *testing.T) {
	tests := []struct {
		name     string
		patterns []string
		path     string
		isDir    bool
		want     bool
	}{{
		name:     "basic glob at root",
		patterns: []string{"*.tmp"},
		path:     "a.tmp",
		isDir:    false,
		want:     true,
	}, {
		name:     "deep path",
		patterns: []string{"*.tmp"},
		path:     "src/deep/a.tmp",
		isDir:    false,
		want:     true,
	}, {
		name:     "non-match",
		patterns: []string{"*.tmp"},
		path:     "a.txt",
		isDir:    false,
		want:     false,
	}, {
	name: "directory only rule",

		}

	for _, tt := range tests {
		m := mustCompile(t, tt.patterns...)
		if got := m.match(tt.path, tt.isDir); got != tt.want {
			t.Errorf("match(%q, isDir=%v) with %v = %v, want %v", tt.path, tt.isDir, tt.patterns, got, tt.want)
		}
	}
}
