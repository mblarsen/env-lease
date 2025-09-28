package daemon

import (
	"context"
	"github.com/mblarsen/env-lease/internal/config"
	"github.com/mblarsen/env-lease/internal/ipc"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
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