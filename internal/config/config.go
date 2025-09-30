package config

import (
	"fmt"
	"github.com/BurntSushi/toml"
	"time"
)

// Config represents the structure of the env-lease.toml file.
type Config struct {
	Lease []Lease `toml:"lease"`
}

// Lease represents a single lease block in the config.
type Lease struct {
	Source        string `toml:"source"`
	Destination   string `toml:"destination"`
	Duration      string `toml:"duration"`
	LeaseType     string `toml:"lease_type"`
	Variable      string `toml:"variable"`
	Format        string   `toml:"format"`
	Transform     []string `toml:"transform"`
	FileMode      string   `toml:"file_mode"`
	OpAccount     string `toml:"op_account"`
	ExpiresAt     time.Time
	OrphanedSince *time.Time
	ConfigFile    string
	ParentSource  string
}

// Load reads a TOML file from the given path, validates it, and returns a Config struct.
func Load(path string) (*Config, error) {
	var rawConfig struct {
		Lease []struct {
			Source      string   `toml:"source"`
			Destination string   `toml:"destination"`
			Duration    string   `toml:"duration"`
			LeaseType   string   `toml:"lease_type"`
			Variable    string   `toml:"variable"`
			Format      string   `toml:"format"`
			Transform   []string `toml:"transform"`
			Encoding    string   `toml:"encoding"` // Keep for backward compatibility
			FileMode    string   `toml:"file_mode"`
			OpAccount   string   `toml:"op_account"`
		} `toml:"lease"`
	}

	if _, err := toml.DecodeFile(path, &rawConfig); err != nil {
		return nil, err
	}

	var config Config
	config.Lease = make([]Lease, len(rawConfig.Lease))

	for i, rawLease := range rawConfig.Lease {
		config.Lease[i] = Lease{
			Source:      rawLease.Source,
			Destination: rawLease.Destination,
			Duration:    rawLease.Duration,
			LeaseType:   rawLease.LeaseType,
			Variable:    rawLease.Variable,
			Format:      rawLease.Format,
			Transform:   rawLease.Transform,
			FileMode:    rawLease.FileMode,
			OpAccount:   rawLease.OpAccount,
		}

		// Backward compatibility for encoding
		if len(config.Lease[i].Transform) == 0 && rawLease.Encoding == "base64" {
			config.Lease[i].Transform = []string{"base64-encode"}
		}

		// Set default lease type
		if config.Lease[i].LeaseType == "" {
			config.Lease[i].LeaseType = "env"
		}

		// Validate required fields
		lease := &config.Lease[i]
		if lease.Source == "" {
			return nil, fmt.Errorf("lease %d: source is required", i)
		}

		if lease.LeaseType == "env" || lease.LeaseType == "file" {
			if lease.Destination == "" {
				return nil, fmt.Errorf("lease %d: destination is required for lease_type '%s'", i, lease.LeaseType)
			}
		}

		if lease.LeaseType == "env" || lease.LeaseType == "shell" {
			if lease.Variable == "" {
				return nil, fmt.Errorf("lease %d: variable is required for lease_type '%s'", i, lease.LeaseType)
			}
		}
	}

	return &config, nil
}
