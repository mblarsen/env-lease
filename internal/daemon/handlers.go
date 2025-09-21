package daemon

import (
	"encoding/json"
	"fmt"
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
		key := fmt.Sprintf("%s;%s", l.Source, l.Destination) // Just an example key
		d.state.Leases[key] = Lease{
			ExpiresAt:   d.clock.Now().Add(duration),
			Source:      l.Source,
			Destination: l.Destination,
			LeaseType:   l.LeaseType,
			Variable:    l.Variable,
		}
	}

	return nil, nil
}

func (d *Daemon) handleRevoke(payload []byte) ([]byte, error) {
	// In a real implementation, we would revoke specific leases.
	// For now, we'll just clear all leases.
	d.state.Leases = make(map[string]Lease)
	return nil, nil
}

func (d *Daemon) handleStatus(payload []byte) ([]byte, error) {
	return json.Marshal(d.state)
}
