package daemon

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/mblarsen/env-lease/internal/config"
	"github.com/mblarsen/env-lease/internal/ipc"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
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
		Source:      "onepassword://vault/item/field1",
		Destination: "/tmp/file1",
		Variable:    "VAR1",
		LeaseType:   "env",
		ConfigFile:  configFile.Name(),
	}
	state.Leases["lease2"] = &config.Lease{
		Source:      "onepassword://vault/item/field2", // This one will be removed from config
		Destination: "/tmp/file2",
		Variable:    "VAR2",
		LeaseType:   "env",
		ConfigFile:  configFile.Name(),
	}
	state.Leases["lease3"] = &config.Lease{
		Source:      "onepassword://vault/item/field3", // This one will be removed from config
		Destination: "/tmp/file3",
		Variable:    "VAR3",
		LeaseType:   "env",
		ConfigFile:  configFile.Name(),
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

func TestDaemon_revokeOrphanedLeases_RevokesRemovedSiblingWithSameSource(t *testing.T) {
	state := NewState()
	clock := &mockClock{now: time.Now()}
	revoker := &mockRevoker{}
	notifier := &mockNotifier{}

	stateFile, err := os.CreateTemp("", "env-lease-state-*.json")
	require.NoError(t, err)
	defer os.Remove(stateFile.Name())

	daemon := NewDaemon(state, stateFile.Name(), clock, nil, revoker, notifier)

	configFile, err := os.CreateTemp("", "env-lease-*.toml")
	require.NoError(t, err)
	defer os.Remove(configFile.Name())

	tempDir := t.TempDir()
	sharedSource := "onepassword://vault/item/shared"
	destinationA := filepath.Join(tempDir, "a.env")
	destinationB := filepath.Join(tempDir, "b.env")

	_, err = configFile.WriteString(fmt.Sprintf(`
[[lease]]
source = %q
destination = %q
duration = "1h"
lease_type = "env"
variable = "VAR_A"

[[lease]]
source = %q
destination = %q
duration = "1h"
lease_type = "env"
variable = "VAR_B"
`, sharedSource, destinationA, sharedSource, destinationB))
	require.NoError(t, err)
	require.NoError(t, configFile.Close())

	state.Leases["lease-a"] = &config.Lease{
		Source:      sharedSource,
		Destination: destinationA,
		Variable:    "VAR_A",
		LeaseType:   "env",
		ConfigFile:  configFile.Name(),
	}
	state.Leases["lease-b"] = &config.Lease{
		Source:      sharedSource,
		Destination: destinationB,
		Variable:    "VAR_B",
		LeaseType:   "env",
		ConfigFile:  configFile.Name(),
	}

	err = os.WriteFile(configFile.Name(), []byte(fmt.Sprintf(`
[[lease]]
source = %q
destination = %q
duration = "1h"
lease_type = "env"
variable = "VAR_A"
`, sharedSource, destinationA)), 0644)
	require.NoError(t, err)

	daemon.revokeOrphanedLeases()

	assert.Equal(t, 1, revoker.RevokeCount, "only removed sibling lease should be revoked")
	require.Len(t, revoker.revoked, 1)
	assert.Equal(t, "VAR_B", revoker.revoked[0].Variable)
	assert.Equal(t, destinationB, revoker.revoked[0].Destination)
	assert.Contains(t, state.Leases, "lease-a")
	assert.NotContains(t, state.Leases, "lease-b")
}

func TestDaemon_revokeOrphanedLeases_DoesNotRevokeUnchangedRelativeDestination(t *testing.T) {
	state := NewState()
	clock := &mockClock{now: time.Now()}
	revoker := &mockRevoker{}
	notifier := &mockNotifier{}

	stateFile, err := os.CreateTemp("", "env-lease-state-*.json")
	require.NoError(t, err)
	defer os.Remove(stateFile.Name())

	daemon := NewDaemon(state, stateFile.Name(), clock, nil, revoker, notifier)

	configFile, err := os.CreateTemp("", "env-lease-*.toml")
	require.NoError(t, err)
	defer os.Remove(configFile.Name())

	source := "onepassword://vault/item/relative"
	relativeDestination := ".env"
	_, err = configFile.WriteString(fmt.Sprintf(`
[[lease]]
source = %q
destination = %q
duration = "1h"
lease_type = "env"
variable = "API_KEY"
`, source, relativeDestination))
	require.NoError(t, err)
	require.NoError(t, configFile.Close())

	state.Leases["relative"] = &config.Lease{
		Source:      source,
		Destination: filepath.Join(filepath.Dir(configFile.Name()), relativeDestination),
		LeaseType:   "env",
		Variable:    "API_KEY",
		ConfigFile:  configFile.Name(),
	}

	daemon.revokeOrphanedLeases()

	assert.Equal(t, 0, revoker.RevokeCount)
	assert.Contains(t, state.Leases, "relative")
}

func TestDaemon_revokeOrphanedLeases_PreservesExplodeChildrenWhenConfigUnchanged(t *testing.T) {
	state := NewState()
	clock := &mockClock{now: time.Now()}
	revoker := &mockRevoker{}
	notifier := &mockNotifier{}

	stateFile, err := os.CreateTemp("", "env-lease-state-*.json")
	require.NoError(t, err)
	defer os.Remove(stateFile.Name())

	daemon := NewDaemon(state, stateFile.Name(), clock, nil, revoker, notifier)

	configFile, err := os.CreateTemp("", "env-lease-*.toml")
	require.NoError(t, err)
	defer os.Remove(configFile.Name())

	source := "onepassword://vault/item/exploded"
	relativeDestination := ".env.exploded"
	_, err = configFile.WriteString(fmt.Sprintf(`
[[lease]]
source = %q
destination = %q
duration = "1h"
lease_type = "env"
transform = ["json", "explode"]
`, source, relativeDestination))
	require.NoError(t, err)
	require.NoError(t, configFile.Close())

	destination := filepath.Join(filepath.Dir(configFile.Name()), relativeDestination)
	parent := parentLeaseIdentity(source, destination)

	state.Leases["parent"] = &config.Lease{
		Source:      source,
		Destination: destination,
		LeaseType:   "env",
		Variable:    "",
		ConfigFile:  configFile.Name(),
	}
	state.Leases["child-key1"] = &config.Lease{
		Source:       source,
		Destination:  destination,
		LeaseType:    "env",
		Variable:     "KEY1",
		ParentSource: parent,
		ConfigFile:   configFile.Name(),
	}
	state.Leases["child-key2"] = &config.Lease{
		Source:       source,
		Destination:  destination,
		LeaseType:    "env",
		Variable:     "KEY2",
		ParentSource: parent,
		ConfigFile:   configFile.Name(),
	}

	daemon.revokeOrphanedLeases()

	assert.Equal(t, 0, revoker.RevokeCount)
	assert.Contains(t, state.Leases, "parent")
	assert.Contains(t, state.Leases, "child-key1")
	assert.Contains(t, state.Leases, "child-key2")
}

func TestDaemon_ShutdownRevokesLeasesAndClearsState(t *testing.T) {
	tempDir := t.TempDir()
	socketPath := filepath.Join(os.TempDir(), fmt.Sprintf("env-lease-shutdown-%d.sock", time.Now().UnixNano()))
	statePath := filepath.Join(tempDir, "state.json")

	state := NewState()
	state.Leases["env"] = &config.Lease{
		Source:      "onepassword://vault/item/env",
		Destination: "/tmp/env",
		LeaseType:   "env",
		Variable:    "ENV_VAR",
	}
	state.Leases["file"] = &config.Lease{
		Source:      "onepassword://vault/item/file",
		Destination: "/tmp/file",
		LeaseType:   "file",
	}
	state.Leases["shell"] = &config.Lease{
		Source:    "onepassword://vault/item/shell",
		LeaseType: "shell",
	}
	envFile := filepath.Join(tempDir, "shutdown.env")
	require.NoError(t, os.WriteFile(envFile, []byte("ENV_VAR=value\n"), 0644))
	fileLease := filepath.Join(tempDir, "shutdown.txt")
	require.NoError(t, os.WriteFile(fileLease, []byte("secret"), 0644))

	state.Leases["env"].Destination = envFile
	state.Leases["file"].Destination = fileLease

	require.NoError(t, state.SaveState(statePath))

	server, err := ipc.NewServer(socketPath, []byte("graceful-shutdown-secret"))
	require.NoError(t, err)
	t.Cleanup(func() { os.Remove(socketPath) })

	revoker := &FileRevoker{}
	notifier := &mockNotifier{}
	d := NewDaemon(state, statePath, &RealClock{}, server, revoker, notifier)

	err = d.Shutdown()
	require.NoError(t, err)

	if d.state != nil {
		assert.Empty(t, d.state.Leases, "daemon state should be cleared after shutdown")
		assert.Empty(t, d.state.RetryQueue, "daemon retry queue should be cleared after shutdown")
	}

	reloaded, err := LoadState(statePath)
	require.NoError(t, err)
	assert.Empty(t, reloaded.Leases, "state file should be cleared and persisted")
	assert.Empty(t, reloaded.RetryQueue, "retry queue should be cleared during shutdown")

	_, err = os.Stat(fileLease)
	assert.True(t, os.IsNotExist(err), "file lease should be removed during shutdown")

	envContent, err := os.ReadFile(envFile)
	require.NoError(t, err)
	assert.Contains(t, string(envContent), "ENV_VAR=")
	assert.NotContains(t, string(envContent), "value", "env variable should be cleared during shutdown")
}

func TestDaemon_processRetryQueue_PersistsBackoffUpdate(t *testing.T) {
	tempDir := t.TempDir()
	statePath := filepath.Join(tempDir, "state.json")
	now := time.Now()

	state := NewState()
	state.RetryQueue = []RetryItem{
		{
			Lease: &config.Lease{
				Source:      "onepassword://vault/item/retry",
				Destination: filepath.Join(tempDir, "retry.env"),
				LeaseType:   "env",
			},
			Attempts:       1,
			NextRetryTime:  now.Add(-1 * time.Second),
			InitialFailure: now.Add(-1 * time.Minute),
		},
	}
	require.NoError(t, state.SaveState(statePath))

	revoker := &mockRevoker{RevokeFunc: func(lease *config.Lease) error {
		return fmt.Errorf("revoke failed")
	}}
	d := NewDaemon(state, statePath, &mockClock{now: now}, nil, revoker, nil)

	d.processRetryQueue()

	require.Len(t, state.RetryQueue, 1)
	assert.Equal(t, 2, state.RetryQueue[0].Attempts)
	assert.WithinDuration(t, now.Add(4*time.Second), state.RetryQueue[0].NextRetryTime, time.Millisecond)

	reloaded, err := LoadState(statePath)
	require.NoError(t, err)
	require.Len(t, reloaded.RetryQueue, 1)
	assert.Equal(t, 2, reloaded.RetryQueue[0].Attempts)
	assert.WithinDuration(t, now.Add(4*time.Second), reloaded.RetryQueue[0].NextRetryTime, time.Millisecond)
}

func TestDaemon_processRetryQueue_PersistsSuccessfulRemoval(t *testing.T) {
	tempDir := t.TempDir()
	statePath := filepath.Join(tempDir, "state.json")
	now := time.Now()

	state := NewState()
	state.RetryQueue = []RetryItem{
		{
			Lease: &config.Lease{
				Source:      "onepassword://vault/item/retry-success",
				Destination: filepath.Join(tempDir, "retry-success.env"),
				LeaseType:   "env",
			},
			Attempts:       1,
			NextRetryTime:  now.Add(-1 * time.Second),
			InitialFailure: now.Add(-1 * time.Minute),
		},
	}
	require.NoError(t, state.SaveState(statePath))

	d := NewDaemon(state, statePath, &mockClock{now: now}, nil, &mockRevoker{}, nil)

	d.processRetryQueue()

	assert.Empty(t, state.RetryQueue)

	reloaded, err := LoadState(statePath)
	require.NoError(t, err)
	assert.Empty(t, reloaded.RetryQueue)
}
