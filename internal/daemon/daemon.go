package daemon

import (
	"context"
	"log/slog"
	"os"
	"os/signal"
	"sync"
	"syscall"
	"time"

	"github.com/mblarsen/env-lease/internal/ipc"
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
	lifecycle *Lifecycle
	mu        sync.Mutex
}

// NewDaemon creates a new daemon.
func NewDaemon(state *State, statePath string, clock Clock, ipcServer *ipc.Server, revoker Revoker, notifier Notifier) *Daemon {
	lifecycle := NewLifecycle(state, statePath, clock, revoker, notifier)
	return &Daemon{
		state:     lifecycle.state,
		clock:     clock,
		ipcServer: ipcServer,
		lifecycle: lifecycle,
	}
}

// Run starts the daemon's main loop.
func (d *Daemon) Run(ctx context.Context) error {
	// Set up a channel to listen for OS signals
	sigs := make(chan os.Signal, 1)
	signal.Notify(sigs, os.Interrupt, syscall.SIGTERM)
	defer signal.Stop(sigs)

	go d.ipcServer.Listen(d.handleIPC)

	d.revokeExpiredLeases()
	d.processRetryQueue()
	d.cleanupOrphanedLeases()

	ticker := d.clock.Ticker(1 * time.Second)
	defer ticker.Stop()

	cleanupTicker := d.clock.Ticker(24 * time.Hour)
	defer cleanupTicker.Stop()

	lastCheckTime := d.clock.Now()
	for {
		slog.Debug("Daemon run loop tick")

		// If the time since the last check is greater than a threshold, force an
		// expiration check. This handles cases where the system has been asleep
		// and the ticker may not have fired.
		if d.clock.Now().Sub(lastCheckTime) > 5*time.Second {
			slog.Info("Detected time jump, forcing expiration check")
			d.revokeExpiredLeases()
			d.processRetryQueue()
			d.revokeOrphanedLeases()
		}

		select {
		case <-ticker.C:
			d.revokeExpiredLeases()
			d.processRetryQueue()
			d.revokeOrphanedLeases()
		case <-cleanupTicker.C:
			d.cleanupOrphanedLeases()
		case sig := <-sigs:
			slog.Info("Received shutdown signal, beginning graceful shutdown", "signal", sig)
			return d.Shutdown()
		case <-ctx.Done():
			slog.Debug("Parent context cancelled, initiating shutdown...")
			return d.Shutdown()
		}
		lastCheckTime = d.clock.Now()
	}
}

func (d *Daemon) revokeOrphanedLeases() {
	d.mu.Lock()
	defer d.mu.Unlock()
	d.lifecycle.ReconcileConfigLeases()
}

func (d *Daemon) cleanupOrphanedLeases() {
	d.mu.Lock()
	defer d.mu.Unlock()
	d.lifecycle.MarkOrphanedLeases()
}

func (d *Daemon) revokeExpiredLeases() {
	d.mu.Lock()
	defer d.mu.Unlock()
	d.lifecycle.RevokeExpired()
}

func (d *Daemon) processRetryQueue() {
	d.mu.Lock()
	defer d.mu.Unlock()
	d.lifecycle.ProcessRetryQueue()
}

func (d *Daemon) Shutdown() error {
	slog.Info("Starting graceful shutdown: revoking active leases and clearing state.")

	if d.ipcServer != nil {
		if err := d.ipcServer.Close(); err != nil {
			slog.Error("Failed to close IPC server during shutdown", "err", err)
		}
		d.ipcServer = nil
	}

	d.mu.Lock()
	err := d.lifecycle.Shutdown()
	d.state = d.lifecycle.state
	d.mu.Unlock()
	return err
}
