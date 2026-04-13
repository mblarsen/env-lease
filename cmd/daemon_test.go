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

	state, err := loadDaemonState(statePath)

	require.NoError(t, err)
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

	loaded, err := loadDaemonState(statePath)

	require.NoError(t, err)
	require.NotNil(t, loaded)
	require.Contains(t, loaded.Leases, "lease1")
	assert.Equal(t, expected.Leases["lease1"].Source, loaded.Leases["lease1"].Source)
}

func TestLoadDaemonStateReturnsErrorForUnreadableState(t *testing.T) {
	statePath := t.TempDir()

	state, err := loadDaemonState(statePath)

	require.Error(t, err)
	assert.Nil(t, state)
	assert.Contains(t, err.Error(), "failed to load daemon state")
	assert.NotContains(t, err.Error(), "malformed")
}
