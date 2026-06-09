// Package config contains the configurations for filesync_g
package config

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"

	"github.com/BurntSushi/toml"
)

type Config struct {
	Ignore  []string
	matcher *ignoreMatcher
}

const (
	DefaultSource = "."
	ConfigFile    = "filesync.toml"
	ManifestDir   = ".filesync"
	ManifestFile  = ManifestDir + "/state.json"
)

// Sentinel errors returned by [Load], for callers and tests to match with
// [errors.Is]. Each wraps the underlying cause for context.
var (
	// ErrMissingConfig means no config file exists at the expected path.
	ErrMissingConfig = errors.New("config file not found")
	// ErrInvalidConfig means the config file exists but could not be parsed as TOML.
	ErrInvalidConfig = errors.New("invalid config file")
	// ErrInvalidIgnorePattern means an ignore entry is not a valid pattern.
	ErrInvalidIgnorePattern = errors.New("invalid ignore pattern")
)

// Load parses the config file as TOML, then compiles the ignore list to
// ensure the entires are valid and to process them to be optimized when executing ShouldIgnore
func Load(source string) (*Config, error) {
	path := filepath.Join(source, ConfigFile)

	if _, err := os.Stat(path); err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil, fmt.Errorf("%w: %s", ErrMissingConfig, path)
		}
		return nil, fmt.Errorf("config: stat %s: %w", path, err)
	}

	var config Config
	if _, err := toml.DecodeFile(path, &config); err != nil {
		return nil, fmt.Errorf("%w: %s: %v", ErrInvalidConfig, path, err)
	}

	// Built-in ignores
	ignore := append([]string{ConfigFile, ManifestDir + "/"}, config.Ignore...)

	m, err := compile(ignore)
	if err != nil {
		return nil, err
	}
	config.matcher = m

	return &config, nil
}

// String renders the config for display, omitting the internal matcher.
func (c *Config) String() string {
	return fmt.Sprintf("Config{ignore: %v}", c.Ignore)
}

// ShouldIgnore uses [path.Match] rules for comparing ignore values against files to see if
// they should be ignored. If a value is not anchored (does not contain "/"), it is compared
// against the base of the paths
func (c *Config) ShouldIgnore(path string, isDir bool) bool {
	if c.matcher == nil {
		return false
	}
	return c.matcher.match(path, isDir)
}
