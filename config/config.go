package config

import (
	"os"
	"path/filepath"

	"gopkg.in/yaml.v3"
)

const Filename = ".c4git.yaml"

// Config holds c4git configuration.
type Config struct {
	Stores   []StoreConfig `yaml:"stores"`
	Patterns []string      `yaml:"patterns"`
}

// StoreConfig describes a single backing store.
type StoreConfig struct {
	Type string `yaml:"type"`
	Path string `yaml:"path"`
}

// DefaultPatterns returns the default set of media file patterns.
func DefaultPatterns() []string {
	return []string{
		"*.exr",
		"*.dpx",
		"*.mov",
		"*.mp4",
		"*.abc",
		"*.vdb",
		"*.bgeo",
		"*.usd",
		"*.usdc",
		"*.usdz",
	}
}

// Default returns a Config with default values.
func Default() *Config {
	return &Config{
		Stores: []StoreConfig{
			{Type: "directory", Path: ".c4/store/c4"},
		},
		Patterns: DefaultPatterns(),
	}
}

// Load reads .c4git.yaml from dir, returning defaults if absent.
func Load(dir string) (*Config, error) {
	path := filepath.Join(dir, Filename)
	data, err := os.ReadFile(path)
	if os.IsNotExist(err) {
		return Default(), nil
	}
	if err != nil {
		return nil, err
	}
	var cfg Config
	if err := yaml.Unmarshal(data, &cfg); err != nil {
		return nil, err
	}
	return &cfg, nil
}

// Write serializes the config to .c4git.yaml in dir.
func (c *Config) Write(dir string) error {
	data, err := yaml.Marshal(c)
	if err != nil {
		return err
	}
	return os.WriteFile(filepath.Join(dir, Filename), data, 0644)
}
