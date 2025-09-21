package daemon

import (
	"context"
	"time"
)

// Clock is an interface for time-related functions to allow for mocking.
type Clock interface {
	Now() time.Time
	Ticker(d time.Duration) *time.Ticker
}

// RealClock is a real implementation of the Clock interface.
type RealClock struct{}

func (c *RealClock) Now() time.Time {
	return time.Now()
}

func (c *RealClock) Ticker(d time.Duration) *time.Ticker {
	return time.NewTicker(d)
}

// Daemon is the main daemon struct.
type Daemon struct {
	state *State
	clock Clock
	// Other dependencies like a revoker will be added here
}

// NewDaemon creates a new daemon.
func NewDaemon(state *State, clock Clock) *Daemon {
	return &Daemon{
		state: state,
		clock: clock,
	}
}

// Run starts the daemon's main loop.
func (d *Daemon) Run(ctx context.Context) error {
	d.revokeExpiredLeases()

	ticker := d.clock.Ticker(1 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			d.revokeExpiredLeases()
		case <-ctx.Done():
			return nil
		}
	}
}

func (d *Daemon) revokeExpiredLeases() {
	now := d.clock.Now()
	for id, lease := range d.state.Leases {
		if now.After(lease.ExpiresAt) {
			// In a real implementation, we would call a revoker here.
			// For now, we'll just remove the lease from the state.
			delete(d.state.Leases, id)
		}
	}
}
