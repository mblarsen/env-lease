package daemon

import (
	"testing"
	"time"
)

type mockClock struct {
	now time.Time
	// We don't need a real ticker for the test
}

func (m *mockClock) Now() time.Time {
	return m.now
}

func (m *mockClock) Ticker(d time.Duration) *time.Ticker {
	// Return a ticker that will never fire, we'll manually check
	return time.NewTicker(24 * time.Hour)
}

func (m *mockClock) Advance(d time.Duration) {
	m.now = m.now.Add(d)
}

func TestDaemon(t *testing.T) {
	state := NewState()
	clock := &mockClock{now: time.Now()}
	daemon := NewDaemon(state, clock)

	t.Run("startup revocation", func(t *testing.T) {
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

	t.Run("run loop", func(t *testing.T) {
		state.Leases = make(map[string]Lease) // Reset state
		clock.now = time.Now()                // Reset time
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
