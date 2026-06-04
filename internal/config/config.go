// Package config contains the configurations for filesync_g
package config

import (
	"os"

	"github.com/BurntSushi/toml"
)

type Config struct {
	Ignore  []string
	matcher *ignoreMatcher
}

// Load parses the config file as TOML, then compiles the ignore list to
// ensure the entires are valid and to process them to be optimized when executing ShouldIgnore
func Load(path string) (*Config, error) {
	if _, err := os.Stat(path); err != nil {
		return nil, err
	}

	var config Config
	if _, err := toml.DecodeFile(path, &config); err != nil {
		return nil, err
	}

	m, err := compile(config.Ignore)
	if err != nil {
		return nil, err
	}
	config.matcher = m

	return &config, nil
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
