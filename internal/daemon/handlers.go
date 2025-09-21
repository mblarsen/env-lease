package daemon

import (
	"encoding/json"
	"fmt"
	"github.com/mblarsen/env-lease/internal/fileutil"
	"github.com/mblarsen/env-lease/internal/ipc"
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

	for _, l := range req.Leases {
		duration, err := time.ParseDuration(l.Duration)
		if err != nil {
			return nil, fmt.Errorf("invalid duration '%s': %w", l.Duration, err)
		}
		if l.LeaseType == "env" {
			content := fmt.Sprintf(l.Format, l.Variable, l.Value)
			if err := fileutil.AtomicWriteFile(l.Destination, []byte(content+"\n"), 0644); err != nil {
				return nil, err
			}
		}

		key := fmt.Sprintf("%s;%s;%s", l.Source, l.Destination, l.Variable)
		d.state.Leases[key] = Lease{
			ExpiresAt:   d.clock.Now().Add(duration),
			Source:      l.Source,
			Destination: l.Destination,
			LeaseType:   l.LeaseType,
			Variable:    l.Variable,
			Value:       l.Value,
		}
	}

	return nil, nil
}

func (d *Daemon) handleRevoke(_ []byte) ([]byte, error) {
	// TODO: This should only revoke leases for the current project context.
	// For now, it revokes all active leases.
	for id, lease := range d.state.Leases {
		if err := d.revoker.Revoke(lease); err != nil {
			// Don't return the error, try to revoke as many as possible
		}
		delete(d.state.Leases, id)
	}
	return nil, nil
}

func (d *Daemon) handleStatus(_ []byte) ([]byte, error) {
	return json.Marshal(d.state)
}
