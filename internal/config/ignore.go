package config

import (
	"fmt"
	"path"
	"path/filepath"
	"strings"
)

type pattern struct {
	raw      string
	glob     string
	dirOnly  bool
	anchored bool
}

type ignoreMatcher struct {
	patterns []pattern
}

// compile verifies each input string from the slice and saves a [pattern] for each to be used
// in [match]
func compile(ignore []string) (*ignoreMatcher, error) {
	var patterns []pattern
	for _, raw := range ignore {
		glob := strings.TrimSpace(raw)

		glob = filepath.ToSlash(glob)
		dirOnly := strings.HasSuffix(glob, "/")
		if dirOnly {
			glob = strings.TrimSuffix(glob, "/")
		}

		if len(glob) == 0 {
			continue
		}

		anchored := strings.Contains(glob, "/")

		if _, err := path.Match(glob, ""); err != nil {
			return nil, fmt.Errorf("config: invalid ignore pattern %q: %w", glob, err)
		}

		patterns = append(patterns, pattern{
			raw,
			glob,
			dirOnly,
			anchored,
		})
	}

	return &ignoreMatcher{
		patterns,
	}, nil
}

func (m *ignoreMatcher) match(relPath string, isDir bool) bool {
	for _, p := range m.patterns {
		if p.dirOnly && !isDir {
			continue
		}

		target := relPath
		if !p.anchored {
			target = path.Base(relPath)
		}
		if ok, _ := path.Match(p.glob, target); ok {
			return true
		}
	}

	return false
}
