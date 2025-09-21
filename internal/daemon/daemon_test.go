package daemon

import (
	"fmt"
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

func TestDaemon(t *testing.T) {
	state := NewState()
	clock := &mockClock{now: time.Now()}
	revoker := &mockRevoker{}
	daemon := NewDaemon(state, clock, nil, revoker)

	t.Run("startup revocation", func(t *testing.T) {
		revoker.RevokeFunc = func(l Lease) error {
			return nil
		}
		state.Leases["expired"] = Lease{ExpiresAt: clock.Now().Add(-1 * time.Hour)}
		state.Leases["active"] = Lease{ExpiresAt: clock.Now().Add(1 * time.Hour)}

		daemon.revokeExpiredLeases()

		if _, exists := state.Leases["expired"]; exists {
			t.Error("expired lease was not revoked")
		}
		if _, exists := state.Leases["active"]; !exists {
			t.Error("active lease was revoked")
		}
	})

	t.Run("retry logic", func(t *testing.T) {
		state.Leases = make(map[string]Lease) // Reset state
		state.RetryQueue = make([]RetryItem, 0)
		clock.now = time.Now() // Reset time
		lease := Lease{ExpiresAt: clock.Now().Add(-1 * time.Hour)}
		state.Leases["retry-test"] = lease

		revoker.RevokeFunc = func(l Lease) error {
			return fmt.Errorf("revoke failed")
		}

		daemon.revokeExpiredLeases()

		if len(state.Leases) != 0 {
			t.Error("lease should have been removed from main map")
		}
		if len(state.RetryQueue) != 1 {
			t.Fatal("lease should have been added to retry queue")
		}

		// Advance time and check retry
		clock.Advance(3 * time.Second)
		daemon.processRetryQueue()

		if state.RetryQueue[0].Attempts != 2 {
			t.Errorf("expected 2 attempts, got %d", state.RetryQueue[0].Attempts)
		}
	})

	t.Run("run loop", func(t *testing.T) {
		state.Leases = make(map[string]Lease) // Reset state
		state.RetryQueue = make([]RetryItem, 0)
		revoker.RevokeFunc = func(l Lease) error {
			return nil
		}
		clock.now = time.Now() // Reset time
		state.Leases["future"] = Lease{ExpiresAt: clock.Now().Add(100 * time.Millisecond)}

		daemon.revokeExpiredLeases() // Initial check
		if _, exists := state.Leases["future"]; !exists {
			t.Fatal("lease revoked prematurely")
		}

		clock.Advance(200 * time.Millisecond)
		daemon.revokeExpiredLeases() // Manual tick

		if _, exists := state.Leases["future"]; exists {
			t.Error("future lease was not revoked after time passed")
		}
	})
}
