package daemon

import (
	"encoding/json"
	"fmt"
	"log/slog"
	"time"

	"github.com/mblarsen/env-lease/internal/config"
	"github.com/mblarsen/env-lease/internal/ipc"
)

func (d *Daemon) handleIPC(payload []byte) ([]byte, error) {
	var req struct {
		Command string
	}
	if err := json.Unmarshal(payload, &req); err != nil {
		return nil, fmt.Errorf("failed to unmarshal command: %w", err)
	}

	switch req.Command {
	case "grant":
		d.mu.Lock()
		defer d.mu.Unlock()
		return d.handleGrant(payload)
	case "revoke":
		d.mu.Lock()
		defer d.mu.Unlock()
		return d.handleRevoke(payload)
	case "status":
		d.mu.Lock()
		defer d.mu.Unlock()
		return d.handleStatus(payload)
	case "cleanup":
		return d.handleCleanup(payload)
	default:
		return nil, fmt.Errorf("unknown command: %s", req.Command)
	}
}

func (d *Daemon) handleCleanup(payload []byte) ([]byte, error) {
	slog.Debug("Received cleanup request")

	// Run the cleanup logic immediately
	d.cleanupOrphanedLeases()
	d.revokeOrphanedLeases()

	resp := ipc.CleanupResponse{Messages: []string{"Orphaned lease cleanup process completed."}}
	slog.Info("Orphaned lease cleanup process completed.")
	return json.Marshal(resp)
}

func (d *Daemon) handleGrant(payload []byte) ([]byte, error) {
	var req ipc.GrantRequest
	if err := json.Unmarshal(payload, &req); err != nil {
		return nil, fmt.Errorf("failed to unmarshal grant request: %w", err)
	}
	slog.Debug("Received grant request", "leases", len(req.Leases))

	// Revoke any leases that are in the state but not in the request
	activeLeases := d.state.LeasesForConfigFile(req.ConfigFile)
	for key, activeLease := range activeLeases {
		found := false
		for _, reqLease := range req.Leases {
			if activeLease.Source == reqLease.Source && activeLease.Destination == reqLease.Destination && activeLease.Variable == reqLease.Variable {
				found = true
				break
			}
		}
		if !found {
			slog.Info("Revoking lease removed from config", "key", key)
			if err := d.revoker.Revoke(activeLease); err != nil {
				slog.Error("Failed to revoke lease removed from config", "key", key, "err", err)
				// Continue trying to revoke other leases
			}
			delete(d.state.Leases, key)
		}
	}

	for _, l := range req.Leases {
		duration, err := time.ParseDuration(l.Duration)
		if err != nil {
			return nil, fmt.Errorf("invalid duration '%s': %w", l.Duration, err)
		}

		key := fmt.Sprintf("%s;%s;%s", l.Source, l.Destination, l.Variable)
		lease := &config.Lease{
			Source:        l.Source,
			Destination:   l.Destination,
			Duration:      l.Duration,
			LeaseType:     l.LeaseType,
			Variable:      l.Variable,
			Format:        l.Format,
			Transform:     l.Transform,
			FileMode:      l.FileMode,
			OpAccount:     l.OpAccount,
			ExpiresAt:     d.clock.Now().Add(duration),
			OrphanedSince: nil,
			ConfigFile:    req.ConfigFile,
			ParentSource:  l.ParentSource,
		}
		d.state.Leases[key] = lease
		slog.Debug("Adding lease to state", "source", lease.Source, "expires_at", lease.ExpiresAt)
	}

	if err := d.state.SaveState(d.statePath); err != nil {
		slog.Error("Failed to save state after grant", "err", err)
		// Do not return error to client, as the grant itself succeeded
	}

	resp := ipc.GrantResponse{Messages: []string{}}
	actualLeaseCount := 0
	for _, l := range req.Leases {
		if l.LeaseType == "file" || l.Variable != "" {
			actualLeaseCount++
		}
	}
	slog.Info("Granted leases", "count", actualLeaseCount)
	return json.Marshal(resp)
}

func (d *Daemon) handleRevoke(payload []byte) ([]byte, error) {
	var req ipc.RevokeRequest
	if err := json.Unmarshal(payload, &req); err != nil {
		return nil, fmt.Errorf("failed to unmarshal revoke request: %w", err)
	}
	slog.Debug("Received revoke request", "config_file", req.ConfigFile, "all", req.All)

	var count int
	var shellCommands []string

	if len(req.Leases) > 0 {
		for _, l := range req.Leases {
			id := fmt.Sprintf("%s;%s;%s", l.Source, l.Destination, l.Variable)
			if lease, ok := d.state.Leases[id]; ok {
				slog.Debug("Revoking lease", "source", lease.Source)
				if lease.LeaseType == "shell" {
					shellCommands = append(shellCommands, fmt.Sprintf("unset %s", lease.Variable))
					slog.Debug("Ignoring revoker for shell lease type", "id", id)
				} else {
					if err := d.revoker.Revoke(lease); err != nil {
						slog.Error("Failed to revoke lease", "id", id, "err", err)
						// Continue trying to revoke other leases
					}
				}
				delete(d.state.Leases, id)
				count++
			}
		}
	} else {
		for id, lease := range d.state.Leases {
			if req.All || lease.ConfigFile == req.ConfigFile {
				slog.Debug("Revoking lease", "source", lease.Source)
				if lease.LeaseType == "shell" {
					shellCommands = append(shellCommands, fmt.Sprintf("unset %s", lease.Variable))
					slog.Debug("Ignoring revoker for shell lease type", "id", id)
				} else {
					if err := d.revoker.Revoke(lease); err != nil {
						slog.Error("Failed to revoke lease", "id", id, "err", err)
						// Continue trying to revoke other leases
					}
				}
				delete(d.state.Leases, id)
				count++
			}
		}
	}

	if err := d.state.SaveState(d.statePath); err != nil {
		slog.Error("Failed to save state after revoke", "err", err)
	}

	if req.All && count > 0 {
		title := "Leases Revoked"
		message := fmt.Sprintf("Revoked %d leases due to system idle.", count)
		if err := d.notifier.Notify(title, message); err != nil {
			slog.Error("Failed to send notification", "err", err)
		}
	}

	slog.Info("Revoked leases", "count", count, "all", req.All, "project", req.ConfigFile)
	resp := ipc.RevokeResponse{
		Messages:      []string{fmt.Sprintf("Revoked %d leases.", count)},
		ShellCommands: shellCommands,
	}
	return json.Marshal(resp)
}

func (d *Daemon) handleStatus(payload []byte) ([]byte, error) {
	var req ipc.StatusRequest
	if err := json.Unmarshal(payload, &req); err != nil {
		return nil, fmt.Errorf("failed to unmarshal status request: %w", err)
	}

	var leases []ipc.Lease
	for _, l := range d.state.Leases {
		if req.ConfigFile == "" || l.ConfigFile == req.ConfigFile {
			leases = append(leases, ipc.Lease{
				Source:       l.Source,
				Destination:  l.Destination,
				LeaseType:    l.LeaseType,
				Variable:     l.Variable,
				ExpiresAt:    l.ExpiresAt,
				ConfigFile:   l.ConfigFile,
				ParentSource: l.ParentSource,
			})
		}
	}
	resp := ipc.StatusResponse{Leases: leases}
	return json.Marshal(resp)
}
