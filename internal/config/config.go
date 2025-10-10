package config

import (
	"fmt"
	"path/filepath"
	"strings"
	"time"

	"github.com/BurntSushi/toml"
	"github.com/mblarsen/env-lease/internal/fileutil"
)

// Config represents the structure of the env-lease.toml file.
type Config struct {
	Lease []Lease `toml:"lease"`
	Root  string  `toml:"-"`
}

// Lease represents a single lease block in the config.
type Lease struct {
	Source        string     `toml:"source"`
	Destination   string     `toml:"destination"`
	Duration      string     `toml:"duration"`
	LeaseType     string     `toml:"lease_type"`
	Variable      string     `toml:"variable"`
	Format        string     `toml:"format"`
	Transform     []string   `toml:"transform"`
	FileMode      string     `toml:"file_mode"`
	OpAccount     string     `toml:"op_account" json:"op_account,omitempty"`
	ExpiresAt     time.Time  `toml:"-" json:"expires_at"`
	OrphanedSince *time.Time `toml:"-" json:"orphaned_since,omitempty"`
	ConfigFile    string     `toml:"-" json:"config_file"`
	ParentSource  string     `toml:"-" json:"parent_source,omitempty"`
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

	var err error
	absPath, err := filepath.Abs(path)
	if err != nil {
		return nil, fmt.Errorf("could not get absolute path for config: %w", err)
	}

	expandedPath, err := fileutil.ExpandPath(path)
	if err != nil {
		return nil, fmt.Errorf("could not expand config path: %w", err)
	}
	absPath, err = filepath.Abs(expandedPath)
	if err != nil {
		return nil, fmt.Errorf("could not get absolute path for config: %w", err)
	}

	if _, err := toml.DecodeFile(absPath, &rawConfig); err != nil {
		return nil, err
	}

	var config Config
	config.Lease = make([]Lease, len(rawConfig.Lease))
	config.Root = filepath.Dir(absPath)

	for i, rawLease := range rawConfig.Lease {
		expandedDest, err := fileutil.ExpandPath(rawLease.Destination)
		if err != nil {
			return nil, fmt.Errorf("lease %d: could not expand destination path: %w", i, err)
		}

		config.Lease[i] = Lease{
			Source:      rawLease.Source,
			Destination: expandedDest,
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

		if lease.LeaseType == "env" {
			if lease.Destination == "" {
				return nil, fmt.Errorf("lease %d: destination is required for lease_type '%s'", i, lease.LeaseType)
			}
		}

		if lease.LeaseType == "file" {
			if lease.Destination == "" {
				lease.Destination = filepath.Base(lease.Source)
			}
		}

		isExplode := false
		for _, t := range lease.Transform {
			if strings.HasPrefix(t, "explode") {
				isExplode = true
				break
			}
		}

		if (lease.LeaseType == "env" || lease.LeaseType == "shell") && lease.Variable == "" && !isExplode {
			return nil, fmt.Errorf("lease %d: variable is required for lease_type '%s'", i, lease.LeaseType)
		}
	}

	return &config, nil
}
