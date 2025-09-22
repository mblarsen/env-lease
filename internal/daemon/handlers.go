package daemon

import (
	"bufio"
	"encoding/json"
	"fmt"
	"github.com/mblarsen/env-lease/internal/fileutil"
	"github.com/mblarsen/env-lease/internal/ipc"
	"log"
	"os"
	"strings"
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

	var messages []string
	for _, l := range req.Leases {
		duration, err := time.ParseDuration(l.Duration)
		if err != nil {
			return nil, fmt.Errorf("invalid duration '%s': %w", l.Duration, err)
		}

		var created bool
		var writeErr error
		var skipWrite bool 

		switch l.LeaseType {
		case "env":
			content := fmt.Sprintf(l.Format, l.Variable, l.Value)
			if !req.Override {
				if _, statErr := os.Stat(l.Destination); !os.IsNotExist(statErr) {
					file, openErr := os.Open(l.Destination)
					if openErr != nil {
						return nil, openErr
					}

					scanner := bufio.NewScanner(file)
					var foundKey bool
					for scanner.Scan() {
						line := scanner.Text()
						if line == content {
							skipWrite = true
							break
						}
						parts := strings.SplitN(line, "=", 2)
						key := strings.TrimSpace(parts[0])
						key = strings.TrimPrefix(key, "export ")

						if key == l.Variable {
							foundKey = true
						}
					}
					file.Close() 

					if !skipWrite && foundKey {
						return nil, fmt.Errorf("variable '%s' already exists in '%s' with a different value. Use --override to replace.", l.Variable, l.Destination)
					}
				}
			}
			if !skipWrite {
				created, writeErr = fileutil.AtomicWriteFile(l.Destination, []byte(content+"\n"), 0644)
			}
		case "file":
			if !req.Override {
				if _, statErr := os.Stat(l.Destination); !os.IsNotExist(statErr) {
					existingContent, readErr := os.ReadFile(l.Destination)
					if readErr != nil {
						return nil, readErr
					}
					if string(existingContent) == l.Value {
						skipWrite = true
					} else {
						return nil, fmt.Errorf("file '%s' already exists with different content. Use --override to replace.", l.Destination)
					}
				}
			}
			if !skipWrite {
				created, writeErr = fileutil.AtomicWriteFile(l.Destination, []byte(l.Value), 0644)
			}
		}

		if writeErr != nil {
			return nil, writeErr
		}
		if created {
			messages = append(messages, fmt.Sprintf("Created file: %s", l.Destination))
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

	if err := d.state.SaveState(d.statePath); err != nil {
		log.Printf("Failed to save state after grant: %v", err)
		// Do not return error to client, as the grant itself succeeded
	}

	resp := ipc.GrantResponse{Messages: messages}
	log.Printf("Granted %d leases", len(req.Leases))
	return json.Marshal(resp)
}

func (d *Daemon) handleRevoke(_ []byte) ([]byte, error) {
	// TODO: This should only revoke leases for the current project context.
	// For now, it revokes all active leases.
	count := len(d.state.Leases)
	for id, lease := range d.state.Leases {
		if err := d.revoker.Revoke(lease); err != nil {
			// Don't return the error, try to revoke as many as possible
		}
		delete(d.state.Leases, id)
	}

	if err := d.state.SaveState(d.statePath); err != nil {
		log.Printf("Failed to save state after revoke: %v", err)
	}

	log.Printf("Manually revoked %d leases", count)
	return nil, nil
}

func (d *Daemon) handleStatus(_ []byte) ([]byte, error) {
	return json.Marshal(d.state)
}
