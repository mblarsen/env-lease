package config

import (
	"fmt"
	"github.com/BurntSushi/toml"
)

// Config represents the structure of the env-lease.toml file.
type Config struct {
	Lease []Lease `toml:"lease"`
}

// Lease represents a single lease block in the config.
type Lease struct {
	Source      string `toml:"source"`
	Destination string `toml:"destination"`
	Duration    string `toml:"duration"`
	LeaseType   string `toml:"lease_type"`
	Variable    string `toml:"variable"`
	Format      string `toml:"format"`
	Encoding    string `toml:"encoding"`
}

// Load reads a TOML file from the given path, validates it, and returns a Config struct.
func Load(path string) (*Config, error) {
	var config Config
	if _, err := toml.DecodeFile(path, &config); err != nil {
		return nil, err
	}

	for i := range config.Lease {
		// Set default lease type
		if config.Lease[i].LeaseType == "" {
			config.Lease[i].LeaseType = "env"
		}

		// Validate required fields
		if config.Lease[i].Source == "" {
			return nil, fmt.Errorf("lease %d: source is required", i)
		}
		if config.Lease[i].Destination == "" {
			return nil, fmt.Errorf("lease %d: destination is required", i)
		}
	}

	return &config, nil
}
