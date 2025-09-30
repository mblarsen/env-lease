package daemon

import (
	"context"
	"github.com/mblarsen/env-lease/internal/config"
	"github.com/mblarsen/env-lease/internal/ipc"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"os"
	"testing"
	"time"
)

type mockClock struct {
	now time.Time
}
func (m *mockClock) Now() time.Time {
	return m.now
}

func (m *mockClock) Ticker(d time.Duration) *time.Ticker {
	return time.NewTicker(24 * time.Hour)
}

func (m *mockClock) Advance(d time.Duration) {
	m.now = m.now.Add(d)
}

// TODO: This is a placeholder test. A real test would need to capture stdout.
func TestDaemon_Run(t *testing.T) {
	state := NewState()
	clock := &mockClock{now: time.Now()}
	revoker := &mockRevoker{}
	notifier := &mockNotifier{}
	server, err := ipc.NewServer("/tmp/env-lease-test.sock", []byte("secret"))
	if err != nil {
		t.Fatal(err)
	}
	daemon := NewDaemon(state, "/dev/null", clock, server, revoker, notifier)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	go func() {
		time.Sleep(100 * time.Millisecond)
		cancel()
	}()

	err = daemon.Run(ctx)
	if err != nil && err != context.Canceled {
		t.Errorf("unexpected error: %v", err)
	}
}

func TestDaemon_revokeExpiredLeases(t *testing.T) {
	// Arrange
	state := NewState()
	socketPath := "/tmp/env-lease-test.sock"
	clock := &mockClock{}
	revoker := &mockRevoker{}
	notifier := &mockNotifier{}
	server, err := ipc.NewServer(socketPath, []byte("secret"))
	require.NoError(t, err)
	daemon := NewDaemon(state, "/dev/null", clock, server, revoker, notifier)

	// Add a lease that is already expired
	state.Leases["test"] = &config.Lease{
		Source:    "onepassword://vault/item/field",
		ExpiresAt: clock.Now().Add(-1 * time.Hour),
	}

	// Act
	daemon.revokeExpiredLeases()

	// Assert
	assert.Equal(t, 1, revoker.RevokeCount)
	assert.Equal(t, 1, notifier.NotifyCount)
	assert.Equal(t, "Lease Expired", notifier.LastTitle)
		assert.Equal(t, "Lease for onepassword://vault/item/field has expired and was revoked.", notifier.LastMessage)
		assert.Empty(t, state.Leases)
		}
	
func TestDaemon_revokeOrphanedLeases(t *testing.T) {
	// Arrange
	state := NewState()
	socketPath := "/tmp/env-lease-test.sock"
	clock := &mockClock{now: time.Now()}
	revoker := &mockRevoker{}
	notifier := &mockNotifier{}
	server, err := ipc.NewServer(socketPath, []byte("secret"))
	require.NoError(t, err)

	stateFile, err := os.CreateTemp("", "env-lease-state-*.json")
	require.NoError(t, err)
	defer os.Remove(stateFile.Name())

	daemon := NewDaemon(state, stateFile.Name(), clock, server, revoker, notifier)

	// Create a temporary config file
	configFile, err := os.CreateTemp("", "env-lease-*.toml")
	require.NoError(t, err)
	defer os.Remove(configFile.Name())

	_, err = configFile.WriteString(`
[[lease]]
source = "onepassword://vault/item/field1"
destination = "/tmp/file1"
duration = "1h"
lease_type = "env"
variable = "VAR1"

[[lease]]
source = "onepassword://vault/item/field2"
destination = "/tmp/file2"
duration = "1h"
lease_type = "env"
variable = "VAR2"
`)
	require.NoError(t, err)
	configFile.Close()

	// Add leases to the state, pretending they were granted from the config
	state.Leases["lease1"] = &config.Lease{
		Source:     "onepassword://vault/item/field1",
		ConfigFile: configFile.Name(),
	}
	state.Leases["lease2"] = &config.Lease{
		Source:     "onepassword://vault/item/field2", // This one will be removed from config
		ConfigFile: configFile.Name(),
	}
	state.Leases["lease3"] = &config.Lease{
		Source:     "onepassword://vault/item/field3", // This one will be removed from config
		ConfigFile: configFile.Name(),
	}

	// Act: Modify the config file to "remove" lease2 and lease3
	err = os.WriteFile(configFile.Name(), []byte(`
[[lease]]
source = "onepassword://vault/item/field1"
destination = "/tmp/file1"
duration = "1h"
lease_type = "env"
variable = "VAR1"
`),
		0644)
	require.NoError(t, err)

	daemon.revokeOrphanedLeases()

	// Assert
	assert.Equal(t, 2, revoker.RevokeCount, "Revoke should be called for the two removed leases")
	assert.Len(t, state.Leases, 1, "Only one lease should remain in the state")
	assert.NotNil(t, state.Leases["lease1"], "Lease1 should still be in the state")
}