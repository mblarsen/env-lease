package cmd

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/mblarsen/env-lease/internal/config"
	"github.com/mblarsen/env-lease/internal/daemon"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestLoadDaemonStateFallsBackToEmptyStateOnCorruption(t *testing.T) {
	statePath := filepath.Join(t.TempDir(), "state.json")
	require.NoError(t, os.WriteFile(statePath, []byte("{"), 0600))

	state := loadDaemonState(statePath)

	require.NotNil(t, state)
	assert.Empty(t, state.Leases)
	assert.Empty(t, state.RetryQueue)
}

func TestLoadDaemonStateReturnsPersistedStateWhenValid(t *testing.T) {
	statePath := filepath.Join(t.TempDir(), "state.json")

	expected := daemon.NewState()
	expected.Leases["lease1"] = &config.Lease{
		Source:    "onepassword://vault/item/field",
		ExpiresAt: time.Now().Add(time.Hour),
	}
	require.NoError(t, expected.SaveState(statePath))

	loaded := loadDaemonState(statePath)

	require.NotNil(t, loaded)
	require.Contains(t, loaded.Leases, "lease1")
	assert.Equal(t, expected.Leases["lease1"].Source, loaded.Leases["lease1"].Source)
}
