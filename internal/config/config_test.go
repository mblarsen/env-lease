package config

import (
	"os"
	"path/filepath"
	"testing"
)

func TestLoad(t *testing.T) {
	t.Run("valid config", func(t *testing.T) {
		content := `
[[lease]]
source = "op://vault/item/secret"
destination = ".envrc"
duration = "1h"
`
		path := createTempConfig(t, content)
		defer os.Remove(path)

		config, err := Load(path)
		if err != nil {
			t.Fatalf("expected no error, got %v", err)
		}

		if len(config.Lease) != 1 {
			t.Fatalf("expected 1 lease, got %d", len(config.Lease))
		}

		if config.Lease[0].LeaseType != "env" {
			t.Errorf("expected default lease type 'env', got %s", config.Lease[0].LeaseType)
		}
	})

	t.Run("missing required source", func(t *testing.T) {
		content := `
[[lease]]
destination = ".envrc"
duration = "1h"
`
		path := createTempConfig(t, content)
		defer os.Remove(path)

		_, err := Load(path)
		if err == nil {
			t.Fatal("expected an error, got nil")
		}
	})

	t.Run("missing required destination", func(t *testing.T) {
		content := `
[[lease]]
source = "op://vault/item/secret"
duration = "1h"
`
		path := createTempConfig(t, content)
		defer os.Remove(path)

		_, err := Load(path)
		if err == nil {
			t.Fatal("expected an error, got nil")
		}
	})
}

func createTempConfig(t *testing.T, content string) string {
	t.Helper()
	dir := t.TempDir()
	path := filepath.Join(dir, "env-lease.toml")
	if err := os.WriteFile(path, []byte(content), 0644); err != nil {
		t.Fatalf("failed to create temp config file: %v", err)
	}
	return path
}
