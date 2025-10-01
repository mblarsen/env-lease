package daemon

import (
	"encoding/json"
	"fmt"
	"os"
	"time"

	"github.com/mblarsen/env-lease/internal/config"
	"github.com/mblarsen/env-lease/internal/fileutil"
)

// State represents the persistent state of the daemon.
type State struct {
	Leases     map[string]*config.Lease `json:"leases"`
	RetryQueue []RetryItem              `json:"retry_queue"`
}

// RetryItem represents a lease that failed to be revoked.
type RetryItem struct {
	Lease          *config.Lease `json:"lease"`
	Attempts       int           `json:"attempts"`
	NextRetryTime  time.Time     `json:"next_retry_time"`
	InitialFailure time.Time     `json:"initial_failure"`
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
	data, err := json.MarshalIndent(s, "", "  ")
	if err != nil {
		return fmt.Errorf("failed to marshal state: %w", err)
	}
	_, err = fileutil.AtomicWriteFile(path, data, 0600)
	return err
}

func (s *State) LeasesForConfigFile(configFile string) map[string]*config.Lease {
	leases := make(map[string]*config.Lease)
	for key, lease := range s.Leases {
		if lease.ConfigFile == configFile {
			leases[key] = lease
		}
	}
	return leases
}
