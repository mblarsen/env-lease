package daemon

import (
	"encoding/json"
	"fmt"
	"github.com/mblarsen/env-lease/internal/fileutil"
	"github.com/mblarsen/env-lease/internal/ipc"
	"log"
	"os"
	"strconv"
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
			if _, statErr := os.Stat(l.Destination); !os.IsNotExist(statErr) {
				existingContent, readErr := os.ReadFile(l.Destination)
				if readErr != nil {
					return nil, readErr
				}
				lines := strings.Split(string(existingContent), "\n")
				var newLines []string
				var found bool
				for _, line := range lines {
					if strings.TrimSpace(line) == "" {
						newLines = append(newLines, line)
						continue
					}
					parts := strings.SplitN(line, "=", 2)
					key := strings.TrimSpace(parts[0])
					key = strings.TrimPrefix(key, "export ")
					if key == l.Variable {
						// This is the desired state of the line, without comments.
						newContent := fmt.Sprintf(l.Format, l.Variable, l.Value)

						// Extract the current value, ignoring comments and quotes.
						var existingValue string
						if len(parts) > 1 {
							valuePart := strings.SplitN(parts[1], "#", 2)[0]
							valuePart = strings.TrimSpace(valuePart)
							if len(valuePart) >= 2 && valuePart[0] == '"' && valuePart[len(valuePart)-1] == '"' {
								valuePart = valuePart[1 : len(valuePart)-1]
							} else if len(valuePart) >= 2 && valuePart[0] == '\'' && valuePart[len(valuePart)-1] == '\'' {
								valuePart = valuePart[1 : len(valuePart)-1]
							}
							existingValue = valuePart
						}

						// If values are the same, it's an idempotent grant. Skip writing.
						if existingValue == l.Value {
							skipWrite = true
							break
						}

						// If values differ, check for override flag.
						if !req.Override {
							if existingValue != "" {
								return nil, fmt.Errorf("variable '%s' already exists in '%s' with a different value. Use --override to replace.", l.Variable, l.Destination)
							}
						}

						// Preserve comment from original line.
						if commentIndex := strings.Index(line, "#"); commentIndex != -1 {
							comment := line[commentIndex:]
							newContent += " " + strings.TrimSpace(comment)
						}

						newLines = append(newLines, newContent)
						found = true
					} else {
						newLines = append(newLines, line)
					}
				}
				if skipWrite {
					break
				}
				if !found {
					if len(newLines) > 0 && newLines[len(newLines)-1] != "" {
						newLines = append(newLines, "")
					}
					newLines = append(newLines, content)
				}
				content = strings.Join(newLines, "\n")
			}
			if !skipWrite {
				created, writeErr = fileutil.AtomicWriteFile(l.Destination, []byte(content), 0644)
			}

		case "file":
			mode := os.FileMode(0644)
			if l.FileMode != "" {
				parsedMode, err := strconv.ParseUint(l.FileMode, 8, 32)
				if err != nil {
					return nil, fmt.Errorf("invalid file_mode: %w", err)
				}
				mode = os.FileMode(parsedMode)
			}
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
				created, writeErr = fileutil.AtomicWriteFile(l.Destination, []byte(l.Value), mode)
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
	var revokedLeases []ipc.Lease
	count := len(d.state.Leases)
	for id, lease := range d.state.Leases {
		if err := d.revoker.Revoke(lease); err != nil {
			// Don't return the error, try to revoke as many as possible
		}
		delete(d.state.Leases, id)
		revokedLeases = append(revokedLeases, ipc.Lease{
			Source:      lease.Source,
			Destination: lease.Destination,
			LeaseType:   lease.LeaseType,
			Variable:    lease.Variable,
			Value:       lease.Value,
		})
	}

	if err := d.state.SaveState(d.statePath); err != nil {
		log.Printf("Failed to save state after revoke: %v", err)
	}

	log.Printf("Manually revoked %d leases", count)
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
			Value:       l.Value,
		})
	}
	resp := ipc.StatusResponse{Leases: leases}
	return json.Marshal(resp)
}
