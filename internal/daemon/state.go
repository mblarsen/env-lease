package daemon

import (
	"encoding/json"
	"github.com/mblarsen/env-lease/internal/config"
	"os"
	"path/filepath"
	"time"
)

// State represents the persistent state of the daemon.
type State struct {
	Leases     map[string]*config.Lease `json:"leases"`
	RetryQueue []RetryItem              `json:"retry_queue"`
}

// RetryItem represents a lease that failed to be revoked.
type RetryItem struct {
	Lease         *config.Lease `json:"lease"`
	Attempts      int           `json:"attempts"`
	NextRetryTime time.Time     `json:"next_retry_time"`
	InitialFailure time.Time `json:"initial_failure"`
}

// NewState creates a new, empty state.
func NewState() *State {
	return &State{
		Leases:     make(map[string]*config.Lease),
		RetryQueue: make([]RetryItem, 0),
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

	if len(data) == 0 {
		return NewState(), nil
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
