package daemon

import (
	"context"
	"fmt"
	"github.com/mblarsen/env-lease/internal/fileutil"
	"github.com/mblarsen/env-lease/internal/ipc"
	"sync"
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
	state     *State
	clock     Clock
	ipcServer *ipc.Server
	revoker   Revoker
	mu        sync.Mutex
	// Other dependencies will be added here
}

// NewDaemon creates a new daemon.
func NewDaemon(state *State, clock Clock, ipcServer *ipc.Server, revoker Revoker) *Daemon {
	return &Daemon{
		state:     state,
		clock:     clock,
		ipcServer: ipcServer,
		revoker:   revoker,
	}
}

// Run starts the daemon's main loop.
func (d *Daemon) Run(ctx context.Context) error {
	go d.ipcServer.Listen(d.handleIPC)

	d.revokeExpiredLeases()

	ticker := d.clock.Ticker(1 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			d.revokeExpiredLeases()
			d.processRetryQueue()
		case <-ctx.Done():
			return d.ipcServer.Close()
		}
	}
}

func (d *Daemon) processRetryQueue() {
	now := d.clock.Now()
	for i := len(d.state.RetryQueue) - 1; i >= 0; i-- {
		item := d.state.RetryQueue[i]
		if now.After(item.NextRetryTime) {
			err := d.revoker.Revoke(item.Lease)
			if err != nil {
				item.Attempts++
				item.NextRetryTime = now.Add(time.Duration(item.Attempts*2) * time.Second) // Exponential backoff
				d.state.RetryQueue[i] = item

				// Create failure file if necessary
				if now.Sub(item.InitialFailure) > 5*time.Minute {
					failureFile := item.Lease.Destination + ".env-lease-REVOCATION-FAILURE"
					content := fmt.Sprintf("Failed to revoke lease for %s at %s", item.Lease.Source, now.Format(time.RFC3339))
					fileutil.AtomicWriteFile(failureFile, []byte(content), 0644)
				}
			} else {
				// Success, remove from queue
				d.state.RetryQueue = append(d.state.RetryQueue[:i], d.state.RetryQueue[i+1:]...)
			}
		}
	}
}

func (d *Daemon) revokeExpiredLeases() {
	now := d.clock.Now()
	for id, lease := range d.state.Leases {
		if now.After(lease.ExpiresAt) {
			err := d.revoker.Revoke(lease)
			if err != nil {
				d.state.RetryQueue = append(d.state.RetryQueue, RetryItem{
					Lease:         lease,
					Attempts:      1,
					NextRetryTime: now.Add(2 * time.Second),
					InitialFailure: now,
				})
			}
			delete(d.state.Leases, id)
		}
	}
}
