package config

import (
	"os"
	"path/filepath"
	"testing"
)

func TestDefaultConfig(t *testing.T) {
	cfg := Default()
	if len(cfg.Stores) != 1 {
		t.Fatalf("expected 1 store, got %d", len(cfg.Stores))
	}
	if cfg.Stores[0].Type != "directory" {
		t.Fatalf("expected directory store, got %s", cfg.Stores[0].Type)
	}
	if len(cfg.Patterns) == 0 {
		t.Fatal("expected default patterns")
	}
}

func TestLoadMissing(t *testing.T) {
	dir := t.TempDir()
	cfg, err := Load(dir)
	if err != nil {
		t.Fatal(err)
	}
	if len(cfg.Stores) != 1 {
		t.Fatal("missing config should return defaults")
	}
}

func TestWriteAndLoad(t *testing.T) {
	dir := t.TempDir()
	cfg := Default()
	cfg.Patterns = []string{"*.exr", "*.dpx"}

	if err := cfg.Write(dir); err != nil {
		t.Fatal(err)
	}

	loaded, err := Load(dir)
	if err != nil {
		t.Fatal(err)
	}
	if len(loaded.Patterns) != 2 {
		t.Fatalf("expected 2 patterns, got %d", len(loaded.Patterns))
	}
	if loaded.Patterns[0] != "*.exr" {
		t.Fatalf("expected *.exr, got %s", loaded.Patterns[0])
	}

	// Verify file exists.
	if _, err := os.Stat(filepath.Join(dir, Filename)); err != nil {
		t.Fatal(err)
	}
}
