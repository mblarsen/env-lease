package daemon

import (
	"encoding/json"
	"fmt"
	"github.com/mblarsen/env-lease/internal/config"
	"github.com/mblarsen/env-lease/internal/ipc"
	"log/slog"
	"time"
)

func (d *Daemon) handleIPC(payload []byte) ([]byte, error) {
	d.mu.Lock()
	defer d.mu.Unlock()

	var req struct {
		Command string
	}
	if err := json.Unmarshal(payload, &req); err != nil {
		return nil, fmt.Errorf("failed to unmarshal command: %w", err)
	}

	switch req.Command {
	case "grant":
		return d.handleGrant(payload)
	case "revoke":
		return d.handleRevoke(payload)
	case "status":
		return d.handleStatus(payload)
	default:
		return nil, fmt.Errorf("unknown command: %s", req.Command)
	}
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
		}
		d.state.Leases[key] = lease
		slog.Debug("Adding lease to state", "source", lease.Source, "expires_at", lease.ExpiresAt)
	}

	if err := d.state.SaveState(d.statePath); err != nil {
		slog.Error("Failed to save state after grant", "err", err)
		// Do not return error to client, as the grant itself succeeded
	}

	resp := ipc.GrantResponse{Messages: []string{}}
	slog.Info("Granted leases", "count", len(req.Leases))
	return json.Marshal(resp)
}

func (d *Daemon) handleRevoke(payload []byte) ([]byte, error) {
	var req ipc.RevokeRequest
	if err := json.Unmarshal(payload, &req); err != nil {
		return nil, fmt.Errorf("failed to unmarshal revoke request: %w", err)
	}
	slog.Debug("Received revoke request", "config_file", req.ConfigFile)

	var count int
	for id, lease := range d.state.Leases {
		if lease.ConfigFile == req.ConfigFile {
			slog.Debug("Revoking lease", "source", lease.Source)
			if err := d.revoker.Revoke(lease); err != nil {
				slog.Error("Failed to revoke lease", "id", id, "err", err)
				// Continue trying to revoke other leases
			}
			delete(d.state.Leases, id)
			count++
		}
	}

	if err := d.state.SaveState(d.statePath); err != nil {
		slog.Error("Failed to save state after revoke", "err", err)
	}

	slog.Info("Revoked leases for project", "count", count, "project", req.ConfigFile)
	resp := ipc.RevokeResponse{Messages: []string{fmt.Sprintf("Revoked %d leases.", count)}}
	return json.Marshal(resp)
}

func (d *Daemon) handleStatus(_ []byte) ([]byte, error) {
	var leases []ipc.Lease
	for _, l := range d.state.Leases {
		leases = append(leases, ipc.Lease{
			Source:      l.Source,
			Destination: l.Destination,
			LeaseType:   l.LeaseType,
			Variable:    l.Variable,
			ExpiresAt:   l.ExpiresAt,
			ConfigFile:  l.ConfigFile,
		})
	}
	resp := ipc.StatusResponse{Leases: leases}
	return json.Marshal(resp)
}
