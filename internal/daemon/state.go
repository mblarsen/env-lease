package daemon

import (
	"encoding/json"
	"os"
	"path/filepath"
	"time"
)

// State represents the persistent state of the daemon.
type State struct {
	Leases map[string]Lease `json:"leases"`
}

// Lease represents a single active lease.
type Lease struct {
	ExpiresAt time.Time `json:"expires_at"`
	// Other lease details...
	Source      string `json:"source"`
	Destination string `json:"destination"`
	LeaseType   string `json:"lease_type"`
	Variable    string `json:"variable"`
}

// NewState creates a new, empty state.
func NewState() *State {
	return &State{
		Leases: make(map[string]Lease),
	}
}

// LoadState loads the daemon state from a file.
func LoadState(path string) (*State, error) {
	data, err := os.ReadFile(path)
	if os.IsNotExist(err) {
		return NewState(), nil
	}
	if err != nil {
		return nil, err
	}

	var state State
	if err := json.Unmarshal(data, &state); err != nil {
		return nil, err
	}
	return &state, nil
}

// SaveState saves the daemon state to a file.
func (s *State) SaveState(path string) error {
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0700); err != nil {
		return err
	}

	data, err := json.MarshalIndent(s, "", "  ")
	if err != nil {
		return err
	}

	return os.WriteFile(path, data, 0600)
}
