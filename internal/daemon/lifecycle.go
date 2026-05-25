package daemon

import (
	"fmt"
	"log/slog"
	"os"
	"time"

	"github.com/mblarsen/env-lease/internal/config"
	"github.com/mblarsen/env-lease/internal/destination"
	"github.com/mblarsen/env-lease/internal/fileutil"
	"github.com/mblarsen/env-lease/internal/ipc"
	"github.com/mblarsen/env-lease/internal/lease"
)

// Lifecycle owns active Lease state and all Daemon-side Lease lifecycle transitions.
type Lifecycle struct {
	state     *State
	statePath string
	clock     Clock
	revoker   Revoker
	notifier  Notifier
}

// RevokeResult describes the observable outcome of a Revoke lifecycle transition.
type RevokeResult struct {
	Count         int
	ShellCommands []string
}

// NewLifecycle creates a lifecycle module over persistent daemon state.
func NewLifecycle(state *State, statePath string, clock Clock, revoker Revoker, notifier Notifier) *Lifecycle {
	return &Lifecycle{
		state:     normalizeState(state),
		statePath: statePath,
		clock:     clock,
		revoker:   revoker,
		notifier:  notifier,
	}
}

// Grant registers requested leases, reconciling removed leases unless append mode is enabled.
func (l *Lifecycle) Grant(req ipc.GrantRequest) (int, error) {
	if !req.Append {
		l.reconcileGrantRequest(req)
	} else {
		slog.Debug("Grant request in append mode; skipping reconciliation revokes", "config_file", req.ConfigFile)
	}

	for _, ipcLease := range req.Leases {
		duration, err := time.ParseDuration(ipcLease.Duration)
		if err != nil {
			return 0, fmt.Errorf("invalid duration '%s': %w", ipcLease.Duration, err)
		}

		runtimeLease := lease.FromIPC(ipcLease)
		runtimeLease.ExpiresAt = l.clock.Now().Add(duration)
		runtimeLease.OrphanedSince = nil
		runtimeLease.ConfigFile = req.ConfigFile
		key := runtimeLease.Identity()
		l.state.Leases[key] = &runtimeLease
		slog.Debug("Adding lease to state", "source", runtimeLease.Source, "expires_at", runtimeLease.ExpiresAt)
	}

	l.saveState("grant")
	return actualIPCLeaseCount(req.Leases), nil
}

func (l *Lifecycle) reconcileGrantRequest(req ipc.GrantRequest) {
	requested := make(map[string]struct{}, len(req.Leases))
	for _, reqLease := range req.Leases {
		requested[lease.FromIPC(reqLease).Identity()] = struct{}{}
	}

	for key, activeLease := range l.state.LeasesForConfigFile(req.ConfigFile) {
		if _, found := requested[activeLease.Identity()]; found {
			continue
		}

		slog.Info("Revoking lease removed from config", "key", key)
		if _, err := l.revokeDestination(activeLease); err != nil {
			slog.Error("Failed to revoke lease removed from config", "key", key, "err", err)
		}
		delete(l.state.Leases, key)
	}
}

// Revoke revokes selected leases, all leases, or leases for one config file.
func (l *Lifecycle) Revoke(req ipc.RevokeRequest) (RevokeResult, error) {
	result := RevokeResult{}

	if len(req.Leases) > 0 {
		for _, ipcLease := range req.Leases {
			id := lease.FromIPC(ipcLease).Identity()
			activeLease, ok := l.state.Leases[id]
			if !ok {
				continue
			}
			l.revokeTrackedLease(id, activeLease, &result)
		}
	} else {
		for id, activeLease := range l.state.Leases {
			if req.All || activeLease.ConfigFile == req.ConfigFile {
				l.revokeTrackedLease(id, activeLease, &result)
			}
		}
	}

	l.saveState("revoke")

	if req.All && result.Count > 0 && l.notifier != nil {
		title := "Leases Revoked"
		message := fmt.Sprintf("Revoked %d leases due to system idle.", result.Count)
		if err := l.notifier.Notify(title, message); err != nil {
			slog.Error("Failed to send notification", "err", err)
		}
	}

	return result, nil
}

func (l *Lifecycle) revokeTrackedLease(id string, activeLease *lease.Lease, result *RevokeResult) {
	slog.Debug("Revoking lease", "source", activeLease.Source)
	revoked, err := l.revokeDestination(activeLease)
	if err != nil {
		slog.Error("Failed to revoke lease", "id", id, "err", err)
	}
	result.ShellCommands = append(result.ShellCommands, revoked.ShellCommands...)

	delete(l.state.Leases, id)
	if isActualLease(activeLease) {
		result.Count++
	}
}

// Status returns active leases, optionally filtered by config file.
func (l *Lifecycle) Status(configFile string) []ipc.Lease {
	leases := make([]ipc.Lease, 0, len(l.state.Leases))
	for _, activeLease := range l.state.Leases {
		if configFile == "" || activeLease.ConfigFile == configFile {
			leases = append(leases, ipc.Lease{
				Source:       activeLease.Source,
				Destination:  activeLease.Destination,
				LeaseType:    activeLease.LeaseType,
				Variable:     activeLease.Variable,
				ExpiresAt:    activeLease.ExpiresAt,
				ConfigFile:   activeLease.ConfigFile,
				ParentSource: activeLease.ParentSource,
			})
		}
	}
	return leases
}

// ReconcileConfigLeases revokes active leases that are no longer declared by their Config.
func (l *Lifecycle) ReconcileConfigLeases() {
	slog.Debug("Checking for orphaned leases from config changes...")

	configFiles := make(map[string]struct{})
	for _, activeLease := range l.state.Leases {
		if activeLease.ConfigFile != "" {
			configFiles[activeLease.ConfigFile] = struct{}{}
		}
	}

	stateChanged := false
	for configFile := range configFiles {
		changed := l.reconcileConfigFile(configFile)
		stateChanged = stateChanged || changed
	}

	if stateChanged {
		l.saveState("revoking orphaned leases")
	}
	slog.Debug("Finished checking for orphaned leases.")
}

func (l *Lifecycle) reconcileConfigFile(configFile string) bool {
	resolvedConfigFile, err := config.ResolveConfigFile(configFile)
	if err != nil {
		slog.Warn("Could not resolve config file, skipping", "config", configFile, "err", err)
		return false
	}

	cfg, err := config.Load(resolvedConfigFile, "")
	if err != nil {
		slog.Warn("Config file not found or failed to load; revoking associated leases", "config", configFile, "err", err)
		return l.revokeLeasesForMissingConfig(configFile)
	}

	configLeases, explodeParents := activeConfigIdentities(cfg, configFile)
	stateChanged := false
	for key, activeLease := range l.state.LeasesForConfigFile(configFile) {
		if _, exists := configLeases[activeLease.Identity()]; exists {
			continue
		}
		if activeLease.ParentSource != "" {
			if _, exists := explodeParents[activeLease.ParentSource]; exists {
				continue
			}
		}

		slog.Info("Lease removed from config, revoking", "key", key)
		if _, err := l.revokeDestination(activeLease); err != nil {
			slog.Error("Failed to revoke orphaned lease", "key", key, "err", err)
			continue
		}
		delete(l.state.Leases, key)
		stateChanged = true
	}
	return stateChanged
}

func (l *Lifecycle) revokeLeasesForMissingConfig(configFile string) bool {
	stateChanged := false
	for key, activeLease := range l.state.LeasesForConfigFile(configFile) {
		if _, err := l.revokeDestination(activeLease); err != nil {
			slog.Error("Failed to revoke orphaned lease", "key", key, "err", err)
			continue
		}
		slog.Info("Revoked orphaned lease", "key", key)
		delete(l.state.Leases, key)
		stateChanged = true
	}
	return stateChanged
}

func activeConfigIdentities(cfg *config.Config, configFile string) (map[string]struct{}, map[string]struct{}) {
	leaseSet, normalizeErrs := lease.NormalizePartial(cfg, configFile)
	for _, err := range normalizeErrs {
		slog.Warn("Skipping invalid config lease during orphan check", "config", configFile, "err", err)
	}

	configLeases := make(map[string]struct{}, len(leaseSet.Leases))
	explodeParents := make(map[string]struct{})
	for _, normalizedLease := range leaseSet.Leases {
		configLeases[normalizedLease.Identity()] = struct{}{}
		if normalizedLease.IsExplode() {
			parent := normalizedLease
			parent.Variable = ""
			configLeases[parent.Identity()] = struct{}{}
			explodeParents[parent.ParentIdentity()] = struct{}{}
		}
	}
	return configLeases, explodeParents
}

// MarkOrphanedLeases marks leases whose Config file is missing and purges long-orphaned leases.
func (l *Lifecycle) MarkOrphanedLeases() {
	slog.Debug("Starting orphaned lease cleanup...")

	now := l.clock.Now()
	stateChanged := false

	for id, activeLease := range l.state.Leases {
		if activeLease.ConfigFile == "" {
			continue
		}

		if _, err := os.Stat(activeLease.ConfigFile); os.IsNotExist(err) {
			if activeLease.OrphanedSince == nil {
				slog.Info("Marking lease as orphaned", "id", id, "config_file", activeLease.ConfigFile)
				activeLease.OrphanedSince = &now
				l.state.Leases[id] = activeLease
				stateChanged = true
			}
		} else if activeLease.OrphanedSince != nil {
			slog.Info("Un-marking lease as orphaned", "id", id)
			activeLease.OrphanedSince = nil
			l.state.Leases[id] = activeLease
			stateChanged = true
		}

		if activeLease.OrphanedSince != nil && now.Sub(*activeLease.OrphanedSince) > 30*24*time.Hour {
			slog.Info("Purging lease orphaned for more than 30 days", "id", id)
			if _, err := l.revokeDestination(activeLease); err != nil {
				slog.Error("Failed to revoke purged lease", "id", id, "err", err)
			}
			delete(l.state.Leases, id)
			stateChanged = true
		}
	}

	if stateChanged {
		l.saveState("cleaning up orphaned leases")
	}
	slog.Debug("Finished cleaning up orphaned leases.")
}

// RevokeExpired revokes expired active leases and queues failed revocations for retry.
func (l *Lifecycle) RevokeExpired() {
	slog.Debug("Checking for expired leases...")

	now := l.clock.Now()
	stateChanged := false
	for id, activeLease := range l.state.Leases {
		if !now.After(activeLease.ExpiresAt) {
			continue
		}

		if _, err := l.revokeDestination(activeLease); err != nil {
			slog.Error("Failed to revoke lease, adding to retry queue", "id", id, "err", err)
			l.state.RetryQueue = append(l.state.RetryQueue, RetryItem{
				Lease:          activeLease,
				Attempts:       1,
				NextRetryTime:  now.Add(2 * time.Second),
				InitialFailure: now,
			})
		} else {
			slog.Info("Lease expired and was revoked", "id", id)
			l.notifyLeaseExpired(activeLease)
		}
		delete(l.state.Leases, id)
		stateChanged = true
	}

	if stateChanged {
		l.saveState("lease expiration")
	}
	slog.Debug("Finished checking for expired leases.")
}

func (l *Lifecycle) notifyLeaseExpired(activeLease *lease.Lease) {
	if l.notifier == nil {
		return
	}
	title := "Lease Expired"
	message := fmt.Sprintf("Lease for %s has expired and was revoked.", activeLease.Source)
	if err := l.notifier.Notify(title, message); err != nil {
		slog.Error("Failed to send notification", "err", err)
	}
}

// ProcessRetryQueue retries failed revocations and records persistent failure markers.
func (l *Lifecycle) ProcessRetryQueue() {
	slog.Debug("Processing retry queue...")

	now := l.clock.Now()
	stateChanged := false
	for i := len(l.state.RetryQueue) - 1; i >= 0; i-- {
		item := l.state.RetryQueue[i]
		if !now.After(item.NextRetryTime) {
			continue
		}

		if _, err := l.revokeDestination(item.Lease); err != nil {
			item.Attempts++
			item.NextRetryTime = now.Add(time.Duration(item.Attempts*2) * time.Second)
			l.state.RetryQueue[i] = item
			stateChanged = true
			l.writeRevocationFailureFile(now, item)
			continue
		}

		l.state.RetryQueue = append(l.state.RetryQueue[:i], l.state.RetryQueue[i+1:]...)
		stateChanged = true
	}

	if stateChanged {
		l.saveState("processing retry queue")
	}
	slog.Debug("Finished processing retry queue.")
}

func (l *Lifecycle) writeRevocationFailureFile(now time.Time, item RetryItem) {
	if now.Sub(item.InitialFailure) <= 5*time.Minute {
		return
	}
	failureFile := item.Lease.Destination + ".env-lease-REVOCATION-FAILURE"
	content := fmt.Sprintf("Failed to revoke lease for %s at %s", item.Lease.Source, now.Format(time.RFC3339))
	if _, err := fileutil.AtomicWriteFile(failureFile, []byte(content), 0644); err != nil {
		slog.Error("Failed to write revocation failure file", "path", failureFile, "err", err)
	}
}

// Shutdown revokes tracked leases, clears state, persists the empty state, and removes the state file.
func (l *Lifecycle) Shutdown() error {
	if l.statePath != "" {
		if reloaded, err := LoadState(l.statePath); err != nil {
			slog.Warn("Failed to reload state from disk during shutdown; continuing with in-memory state", "err", err)
		} else {
			l.state = reloaded
		}
	}
	if l.state == nil {
		l.state = NewState()
	}

	leasesToRevoke, retryItems := l.drainStateForShutdown()

	for _, activeLease := range leasesToRevoke {
		if activeLease.LeaseType == lease.TypeShell {
			slog.Debug("Skipping shell lease during shutdown", "source", activeLease.Source)
			continue
		}
		if _, err := l.revokeDestination(activeLease); err != nil {
			slog.Error("Failed to revoke lease during shutdown", "source", activeLease.Source, "destination", activeLease.Destination, "err", err)
		} else {
			slog.Info("Revoked lease during shutdown", "source", activeLease.Source, "destination", activeLease.Destination)
		}
	}

	for _, retry := range retryItems {
		if retry.Lease == nil {
			continue
		}
		if retry.Lease.LeaseType == lease.TypeShell {
			slog.Debug("Skipping shell lease from retry queue during shutdown", "source", retry.Lease.Source)
			continue
		}
		if _, err := l.revokeDestination(retry.Lease); err != nil {
			slog.Error("Failed to revoke lease from retry queue during shutdown", "source", retry.Lease.Source, "destination", retry.Lease.Destination, "err", err)
		} else {
			slog.Info("Revoked lease from retry queue during shutdown", "source", retry.Lease.Source, "destination", retry.Lease.Destination)
		}
	}

	l.state.Leases = make(map[string]*lease.Lease)
	l.state.RetryQueue = nil
	if err := l.state.SaveState(l.statePath); err != nil {
		slog.Error("Failed to save cleared state during shutdown", "err", err)
		return nil
	}

	if l.statePath != "" {
		if removeErr := os.Remove(l.statePath); removeErr != nil && !os.IsNotExist(removeErr) {
			slog.Warn("Failed to remove state file after saving empty state", "err", removeErr)
		} else {
			slog.Info("State cleared and state file removed; shutdown complete.")
		}
	} else {
		slog.Info("State cleared; shutdown complete.")
	}
	return nil
}

func (l *Lifecycle) drainStateForShutdown() ([]*lease.Lease, []RetryItem) {
	leasesToRevoke := make([]*lease.Lease, 0, len(l.state.Leases))
	for key, activeLease := range l.state.Leases {
		if activeLease == nil {
			delete(l.state.Leases, key)
			continue
		}
		leasesToRevoke = append(leasesToRevoke, activeLease)
		delete(l.state.Leases, key)
	}

	retryItems := l.state.RetryQueue
	l.state.RetryQueue = nil
	return leasesToRevoke, retryItems
}

func (l *Lifecycle) revokeDestination(activeLease *lease.Lease) (destination.Revoked, error) {
	if activeLease == nil || l.revoker == nil {
		return destination.Revoked{}, nil
	}
	return l.revoker.Revoke(activeLease)
}

func (l *Lifecycle) saveState(action string) {
	if err := l.state.SaveState(l.statePath); err != nil {
		slog.Error("Failed to save state after "+action, "err", err)
	}
}

func actualIPCLeaseCount(leases []ipc.Lease) int {
	count := 0
	for _, ipcLease := range leases {
		if ipcLease.LeaseType == lease.TypeFile || ipcLease.Variable != "" {
			count++
		}
	}
	return count
}

func isActualLease(activeLease *lease.Lease) bool {
	return activeLease.LeaseType == lease.TypeFile || activeLease.Variable != ""
}
