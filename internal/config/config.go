// Package config contains the configurations for filesync_g
package config

import (
	"os"

	"github.com/BurntSushi/toml"
)

type Config struct {
	Ignore []string
}

func Load(path string) (*Config, error) {
	if _, err := os.Stat(path); err != nil {
		return nil, err
	}

	var config Config
	if _, err := toml.DecodeFile(path, &config); err != nil {
		return nil, err
	}

	return &config, nil
}

func (c *Config) ShouldIgnore(path string, isDir bool) bool {
	return false
}
