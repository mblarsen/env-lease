package daemon

import (
	"github.com/mblarsen/env-lease/internal/config"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestState(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "state.json")

	t.Run("save and load", func(t *testing.T) {
		state := NewState()
		state.Leases["lease1"] = &config.Lease{
			ExpiresAt: time.Now().Add(1 * time.Hour),
			Source:    "test",
		}

		if err := state.SaveState(path); err != nil {
			t.Fatalf("SaveState failed: %v", err)
		}

		loadedState, err := LoadState(path)
		if err != nil {
			t.Fatalf("LoadState failed: %v", err)
		}

		if len(loadedState.Leases) != 1 {
			t.Fatalf("expected 1 lease, got %d", len(loadedState.Leases))
		}
		if loadedState.Leases["lease1"].Source != "test" {
			t.Errorf("expected source 'test', got %s", loadedState.Leases["lease1"].Source)
		}
	})

	t.Run("load non-existent", func(t *testing.T) {
		nonExistentPath := filepath.Join(dir, "non-existent.json")
		state, err := LoadState(nonExistentPath)
		if err != nil {
			t.Fatalf("LoadState failed: %v", err)
		}
		if len(state.Leases) != 0 {
			t.Errorf("expected 0 leases, got %d", len(state.Leases))
		}
	})

	t.Run("load corrupted", func(t *testing.T) {
		corruptedPath := filepath.Join(dir, "corrupted.json")
		if err := os.WriteFile(corruptedPath, []byte("{"), 0600); err != nil {
			t.Fatalf("failed to write corrupted file: %v", err)
		}
		_, err := LoadState(corruptedPath)
		if err == nil {
			t.Fatal("expected an error, got nil")
		}
	})
}
