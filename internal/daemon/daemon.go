package daemon

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"os/signal"
	"sync"
	"syscall"
	"time"

	"github.com/mblarsen/env-lease/internal/config"
	"github.com/mblarsen/env-lease/internal/fileutil"
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
	statePath string
	clock     Clock
	ipcServer *ipc.Server
	revoker   Revoker
	notifier  Notifier
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
		notifier:  notifier,
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
	slog.Debug("Checking for orphaned leases from config changes...")
	d.mu.Lock()
	defer d.mu.Unlock()

	// Gather unique config files from the state
	configFiles := make(map[string]struct{})
	for _, lease := range d.state.Leases {
		if lease.ConfigFile != "" {
			configFiles[lease.ConfigFile] = struct{}{}
		}
	}

	stateChanged := false
	for configFile := range configFiles {
		// Load the current configuration from disk
		resolvedConfigFile, err := config.ResolveConfigFile(configFile)
		if err != nil {
			slog.Warn("Could not resolve config file, skipping", "config", configFile, "err", err)
			continue
		}
		cfg, err := config.Load(resolvedConfigFile, "")
		if err != nil {
			// If config can't be loaded (e.g., deleted), revoke all leases associated with it.
			slog.Warn("Config file not found or failed to load; revoking associated leases", "config", configFile, "err", err)
			for key, lease := range d.state.LeasesForConfigFile(configFile) {
				if lease.LeaseType == "shell" {
					slog.Debug("Ignoring shell lease type in orphaned lease check", "key", key)
					delete(d.state.Leases, key)
					stateChanged = true
					continue
				}
				if err := d.revoker.Revoke(lease); err != nil {
					slog.Error("Failed to revoke orphaned lease", "key", key, "err", err)
				} else {
					slog.Info("Revoked orphaned lease", "key", key)
					delete(d.state.Leases, key)
					stateChanged = true
				}
			}
			continue
		}

		// Create a map of leases defined in the config for efficient lookup
		configLeases := make(map[string]struct{})
		for _, l := range cfg.Lease {
			// The key in the state is a composite of source, destination, and variable.
			// We only need to check the source for existence.
			configLeases[l.Source] = struct{}{}
		}

		// Check active leases against the config
		for key, activeLease := range d.state.LeasesForConfigFile(configFile) {
			if _, exists := configLeases[activeLease.Source]; !exists {
				slog.Info("Lease removed from config, revoking", "key", key)
				if activeLease.LeaseType == "shell" {
					slog.Debug("Ignoring shell lease type in orphaned lease check", "key", key)
					delete(d.state.Leases, key)
					stateChanged = true
					continue
				}
				if err := d.revoker.Revoke(activeLease); err != nil {
					slog.Error("Failed to revoke orphaned lease", "key", key, "err", err)
					// Optionally, add to a retry queue here as well
				} else {
					delete(d.state.Leases, key)
					stateChanged = true
				}
			}
		}
	}

	if stateChanged {
		if err := d.state.SaveState(d.statePath); err != nil {
			slog.Error("Failed to save state after revoking orphaned leases", "err", err)
		}
	}
	slog.Debug("Finished checking for orphaned leases.")
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

	if d.statePath != "" {
		if reloaded, err := LoadState(d.statePath); err != nil {
			slog.Warn("Failed to reload state from disk during shutdown; continuing with in-memory state", "err", err)
		} else {
			d.state = reloaded
		}
	}
	if d.state == nil {
		d.state = NewState()
	}

	leasesToRevoke := make([]*config.Lease, 0, len(d.state.Leases))
	for key, lease := range d.state.Leases {
		if lease == nil {
			delete(d.state.Leases, key)
			continue
		}
		leasesToRevoke = append(leasesToRevoke, lease)
		delete(d.state.Leases, key)
	}

	retryItems := d.state.RetryQueue
	d.state.RetryQueue = nil

	d.mu.Unlock()

	for _, lease := range leasesToRevoke {
		if lease.LeaseType == "shell" {
			slog.Debug("Skipping shell lease during shutdown", "source", lease.Source)
			continue
		}
		if err := d.revoker.Revoke(lease); err != nil {
			slog.Error("Failed to revoke lease during shutdown", "source", lease.Source, "destination", lease.Destination, "err", err)
		} else {
			slog.Info("Revoked lease during shutdown", "source", lease.Source, "destination", lease.Destination)
		}
	}

	for _, retry := range retryItems {
		if retry.Lease == nil {
			continue
		}
		if retry.Lease.LeaseType == "shell" {
			slog.Debug("Skipping shell lease from retry queue during shutdown", "source", retry.Lease.Source)
			continue
		}
		if err := d.revoker.Revoke(retry.Lease); err != nil {
			slog.Error("Failed to revoke lease from retry queue during shutdown", "source", retry.Lease.Source, "destination", retry.Lease.Destination, "err", err)
		} else {
			slog.Info("Revoked lease from retry queue during shutdown", "source", retry.Lease.Source, "destination", retry.Lease.Destination)
		}
	}

	d.mu.Lock()
	d.state.Leases = make(map[string]*config.Lease)
	d.state.RetryQueue = nil
	err := d.state.SaveState(d.statePath)
	d.mu.Unlock()

	if err != nil {
		slog.Error("Failed to save cleared state during shutdown", "err", err)
	} else {
		if d.statePath != "" {
			if removeErr := os.Remove(d.statePath); removeErr != nil && !os.IsNotExist(removeErr) {
				slog.Warn("Failed to remove state file after saving empty state", "err", removeErr)
			} else {
				slog.Info("State cleared and state file removed; shutdown complete.")
			}
		} else {
			slog.Info("State cleared; shutdown complete.")
		}
	}
	return nil
}

func (d *Daemon) cleanupOrphanedLeases() {
	slog.Debug("Starting orphaned lease cleanup...")
	d.mu.Lock()
	defer d.mu.Unlock()

	now := d.clock.Now()
	stateChanged := false

	for id, lease := range d.state.Leases {
		// If a lease doesn't have a config file path, we can't check it.
		if lease.ConfigFile == "" {
			continue
		}

		// Check if the original config file still exists.
		if _, err := os.Stat(lease.ConfigFile); os.IsNotExist(err) {
			// The file is gone, so the lease is an orphan.
			if lease.OrphanedSince == nil {
				slog.Info("Marking lease as orphaned", "id", id, "config_file", lease.ConfigFile)
				lease.OrphanedSince = &now
				d.state.Leases[id] = lease
				stateChanged = true
			}
		} else {
			// The file exists, so if it was marked as orphaned, un-mark it.
			if lease.OrphanedSince != nil {
				slog.Info("Un-marking lease as orphaned", "id", id)
				lease.OrphanedSince = nil
				d.state.Leases[id] = lease
				stateChanged = true
			}
		}

		// Now, check if the lease has been orphaned for too long.
		if lease.OrphanedSince != nil && now.Sub(*lease.OrphanedSince) > 30*24*time.Hour {
			slog.Info("Purging lease orphaned for more than 30 days", "id", id)
			// We can reuse the revokeOrphanedLeases logic, which already handles revocation.
			// Here we just delete it from the state.
			if err := d.revoker.Revoke(lease); err != nil {
				slog.Error("Failed to revoke purged lease", "id", id, "err", err)
			}
			delete(d.state.Leases, id)
			stateChanged = true
		}
	}

	if stateChanged {
		if err := d.state.SaveState(d.statePath); err != nil {
			slog.Error("Failed to save state after cleaning up orphaned leases", "err", err)
		}
	}
	slog.Debug("Finished cleaning up orphaned leases.")
}

func (d *Daemon) revokeExpiredLeases() {
	slog.Debug("Checking for expired leases...")
	d.mu.Lock()
	defer d.mu.Unlock()

	now := d.clock.Now()
	for id, lease := range d.state.Leases {
		if now.After(lease.ExpiresAt) {
			var err error
			if lease.LeaseType != "shell" {
				err = d.revoker.Revoke(lease)
			}

			if err != nil {
				slog.Error("Failed to revoke lease, adding to retry queue", "id", id, "err", err)
				d.state.RetryQueue = append(d.state.RetryQueue, RetryItem{
					Lease:          lease,
					Attempts:       1,
					NextRetryTime:  now.Add(2 * time.Second),
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
	slog.Debug("Finished checking for expired leases.")
}

func (d *Daemon) processRetryQueue() {
	slog.Debug("Processing retry queue...")
	d.mu.Lock()
	defer d.mu.Unlock()

	now := d.clock.Now()
	for i := len(d.state.RetryQueue) - 1; i >= 0; i-- {
		item := d.state.RetryQueue[i]
		if now.After(item.NextRetryTime) {
			var err error
			if item.Lease.LeaseType != "shell" {
				err = d.revoker.Revoke(item.Lease)
			}

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
	slog.Debug("Finished processing retry queue.")
}
