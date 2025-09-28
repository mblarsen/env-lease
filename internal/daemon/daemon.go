package daemon

import (
	"context"
	"fmt"
	"github.com/mblarsen/env-lease/internal/fileutil"
	"github.com/mblarsen/env-lease/internal/ipc"
	"log/slog"
	"os"
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
	statePath string
	clock     Clock
	ipcServer *ipc.Server
	revoker   Revoker
	notifier Notifier
	mu        sync.Mutex
}

// NewDaemon creates a new daemon.
func NewDaemon(state *State, statePath string, clock Clock, ipcServer *ipc.Server, revoker Revoker, notifier Notifier) *Daemon {
	return &Daemon{
		state:     state,
		statePath: statePath,
		clock:     clock,
		ipcServer: ipcServer,
		revoker:   revoker,
		notifier: notifier,
	}
}

// Run starts the daemon's main loop.
func (d *Daemon) Run(ctx context.Context) error {
	go d.ipcServer.Listen(d.handleIPC)

	d.revokeExpiredLeases()
	d.processRetryQueue()
	d.cleanupOrphanedLeases()

	ticker := d.clock.Ticker(1 * time.Second)
	defer ticker.Stop()

	cleanupTicker := d.clock.Ticker(24 * time.Hour)
	defer cleanupTicker.Stop()

	for {
		select {
		case <-ticker.C:
			d.revokeExpiredLeases()
			d.processRetryQueue()
		case <-cleanupTicker.C:
			d.cleanupOrphanedLeases()
		case <-ctx.Done():
			return d.ipcServer.Close()
		}
	}
}

func (d *Daemon) cleanupOrphanedLeases() {
	now := d.clock.Now()
	for id, lease := range d.state.Leases {
		// This is a simplified check. A real implementation would need
		// to know the original config path for each lease.
		configPath := "env-lease.toml" // Placeholder
		if _, err := os.Stat(configPath); os.IsNotExist(err) {
			if lease.OrphanedSince == nil {
				lease.OrphanedSince = &now
				d.state.Leases[id] = lease
			} else if now.Sub(*lease.OrphanedSince) > 30*24*time.Hour {
				delete(d.state.Leases, id)
			}
		} else {
			if lease.OrphanedSince != nil {
				lease.OrphanedSince = nil
				d.state.Leases[id] = lease
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
				slog.Error("Failed to revoke lease, adding to retry queue", "id", id, "err", err)
				d.state.RetryQueue = append(d.state.RetryQueue, RetryItem{
					Lease:         lease,
					Attempts:      1,
					NextRetryTime: now.Add(2 * time.Second),
					InitialFailure: now,
				})
			} else {
				slog.Info("Lease expired and was revoked", "id", id)
				if d.notifier != nil {
					title := "Lease Expired"
					message := fmt.Sprintf("Lease for %s has expired and was revoked.", lease.Source)
					if err := d.notifier.Notify(title, message); err != nil {
						slog.Error("Failed to send notification", "err", err)
					}
				}
			}
			delete(d.state.Leases, id)
			if err := d.state.SaveState(d.statePath); err != nil {
				slog.Error("Failed to save state after lease expiration", "err", err)
			}
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
