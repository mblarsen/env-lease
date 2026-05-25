package daemon

import (
	"encoding/json"
	"fmt"
	"log/slog"

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
		d.mu.Lock()
		defer d.mu.Unlock()
		return d.handleCleanup(payload)
	default:
		return nil, fmt.Errorf("unknown command: %s", req.Command)
	}
}

func (d *Daemon) handleCleanup(payload []byte) ([]byte, error) {
	slog.Debug("Received cleanup request")

	d.lifecycle.MarkOrphanedLeases()
	d.lifecycle.ReconcileConfigLeases()

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

	actualLeaseCount, err := d.lifecycle.Grant(req)
	if err != nil {
		return nil, err
	}

	resp := ipc.GrantResponse{Messages: []string{}}
	slog.Info("Granted leases", "count", actualLeaseCount)
	return json.Marshal(resp)
}

func (d *Daemon) handleRevoke(payload []byte) ([]byte, error) {
	var req ipc.RevokeRequest
	if err := json.Unmarshal(payload, &req); err != nil {
		return nil, fmt.Errorf("failed to unmarshal revoke request: %w", err)
	}
	slog.Debug("Received revoke request", "config_file", req.ConfigFile, "all", req.All)

	result, err := d.lifecycle.Revoke(req)
	if err != nil {
		return nil, err
	}

	slog.Info("Revoked leases", "count", result.Count, "all", req.All, "project", req.ConfigFile)
	resp := ipc.RevokeResponse{
		Messages:      []string{fmt.Sprintf("Revoked %d leases.", result.Count)},
		ShellCommands: result.ShellCommands,
	}
	return json.Marshal(resp)
}

func (d *Daemon) handleStatus(payload []byte) ([]byte, error) {
	var req ipc.StatusRequest
	if err := json.Unmarshal(payload, &req); err != nil {
		return nil, fmt.Errorf("failed to unmarshal status request: %w", err)
	}

	resp := ipc.StatusResponse{Leases: d.lifecycle.Status(req.ConfigFile)}
	return json.Marshal(resp)
}
