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
	}{
		// Unanchored glob: matches the basename at any depth.
		{
			name:     "basic glob at root",
			patterns: []string{"*.tmp"},
			path:     "a.tmp",
			isDir:    false,
			want:     true,
		},
		{
			name:     "glob matches at depth",
			patterns: []string{"*.tmp"},
			path:     "src/deep/a.tmp",
			isDir:    false,
			want:     true,
		},
		{
			name:     "glob non-match",
			patterns: []string{"*.tmp"},
			path:     "a.txt",
			isDir:    false,
			want:     false,
		},

		// Directory-only rule (trailing "/"): only matches directories.
		{
			name:     "dir rule matches dir",
			patterns: []string{"node_modules/"},
			path:     "node_modules",
			isDir:    true,
			want:     true,
		},
		{
			name:     "dir rule ignores file of same name",
			patterns: []string{"node_modules/"},
			path:     "node_modules",
			isDir:    false,
			want:     false,
		},
		{
			name:     "dir rule matches at depth",
			patterns: []string{"node_modules/"},
			path:     "src/dist/node_modules",
			isDir:    true,
			want:     true,
		},

		// Anchored (contains "/"): matches the full relative path exactly.
		{
			name:     "anchored exact match",
			patterns: []string{"build/cache"},
			path:     "build/cache",
			isDir:    true,
			want:     true,
		},
		{
			name:     "anchored does not float to subdirs",
			patterns: []string{"build/cache"},
			path:     "x/build/cache",
			isDir:    true,
			want:     false,
		},
		{
			name:     "anchored does not match children",
			patterns: []string{"build/cache"},
			path:     "build/cache/file.go",
			isDir:    false,
			want:     false, // pruning under a matched dir is scan's SkipDir job, not match's
		},

		// Bare name can substitute for "**/name".
		{
			name:     "bare name matches at any depth",
			patterns: []string{"secrets.json"},
			path:     "a/b/secrets.json",
			isDir:    false,
			want:     true,
		},
		{
			name:     "bare name is a full-segment match",
			patterns: []string{"secrets.json"},
			path:     "a/secrets.json.bak",
			isDir:    false,
			want:     false,
		},

		// Multiple patterns and clean misses.
		{
			name:     "any matching pattern wins",
			patterns: []string{"*.tmp", "*.log"},
			path:     "a.log",
			isDir:    false,
			want:     true,
		},
		{
			name:     "no pattern matches",
			patterns: []string{"*.tmp", "*.log"},
			path:     "a.txt",
			isDir:    false,
			want:     false,
		},
		{
			name:     "empty matcher ignores nothing",
			patterns: []string{},
			path:     "anything",
			isDir:    false,
			want:     false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			m := mustCompile(t, tt.patterns...)
			if got := m.match(tt.path, tt.isDir); got != tt.want {
				t.Errorf("match(%q, isDir=%v) with %v = %v, want %v",
					tt.path, tt.isDir, tt.patterns, got, tt.want)
			}
		})
	}
}
