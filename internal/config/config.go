package config

import (
	"fmt"
	"path/filepath"
	"reflect"
	"strings"
	"time"

	"os"

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
	return loadAndMerge(path, 0)
}

func loadAndMerge(path string, depth int) (*Config, error) {
	if depth > 10 {
		return nil, fmt.Errorf("max include depth exceeded")
	}

	var rawConfig struct {
		Lease []Lease `toml:"lease"`
	}

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
		// Ignore file not found errors for local overrides, which are optional
		if os.IsNotExist(err) && depth > 0 {
			return nil, nil
		}
		return nil, err
	}

	config := Config{
		Lease: rawConfig.Lease,
		Root:  filepath.Dir(absPath),
	}

	for i := range config.Lease {
		lease := &config.Lease[i]
		expandedDest, err := fileutil.ExpandPath(lease.Destination)
		if err != nil {
			return nil, fmt.Errorf("lease %d: could not expand destination path: %w", i, err)
		}
		lease.Destination = expandedDest

		// Backward compatibility for encoding
		if len(lease.Transform) == 0 && lease.Format == "base64" {
			lease.Transform = []string{"base64-encode"}
		}

		// Set default lease type
		if lease.LeaseType == "" {
			lease.LeaseType = "env"
		}

		// Validate required fields
		if lease.Source == "" {
			return nil, fmt.Errorf("lease %d: source is required", i)
		}

		if lease.LeaseType == "env" && lease.Destination == "" {
			return nil, fmt.Errorf("lease %d: destination is required for lease_type '%s'", i, lease.LeaseType)
		}

		if lease.LeaseType == "file" && lease.Destination == "" {
			lease.Destination = filepath.Base(lease.Source)
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

	// Load local override file
	localPath := localOverridePath(absPath)
	localConfig, err := loadAndMerge(localPath, depth+1)
	if err != nil {
		return nil, fmt.Errorf("could not load local config: %w", err)
	}
	mergedConfig := mergeConfigs(&config, localConfig)
	return &mergedConfig, nil
}

func mergeConfigs(base, override *Config) Config {
	if override == nil {
		return *base
	}

	merged := *base

	// Merge simple fields by overriding
	valBase := reflect.ValueOf(&merged).Elem()
	valOverride := reflect.ValueOf(override).Elem()

	for i := 0; i < valOverride.NumField(); i++ {
		field := valOverride.Type().Field(i)
		if field.Name == "Lease" || field.Name == "Root" {
			continue
		}

		valOverrideField := valOverride.Field(i)
		if valOverrideField.IsValid() && !isZero(valOverrideField) {
			valBase.FieldByName(field.Name).Set(valOverrideField)
		}
	}

	// Append leases
	merged.Lease = append(merged.Lease, override.Lease...)

	return merged
}

func isZero(v reflect.Value) bool {
	switch v.Kind() {
	case reflect.Func, reflect.Map, reflect.Slice:
		return v.IsNil()
	case reflect.Array:
		z := true
		for i := 0; i < v.Len(); i++ {
			z = z && isZero(v.Index(i))
		}
		return z
	case reflect.Struct:
		z := true
		for i := 0; i < v.NumField(); i++ {
			z = z && isZero(v.Field(i))
		}
		return z
	}
	// Compare other types directly:
	z := reflect.Zero(v.Type())
	return v.Interface() == z.Interface()
}

func localOverridePath(basePath string) string {
	dir := filepath.Dir(basePath)
	ext := filepath.Ext(basePath)
	baseName := strings.TrimSuffix(filepath.Base(basePath), ext)
	return filepath.Join(dir, fmt.Sprintf("%s.local%s", baseName, ext))
}

// ResolveConfigFile determines the configuration file path based on a predefined
// order of precedence:
//
//  1. --config flag: A path provided via a command-line flag.
//  2. ENV_LEASE_CONFIG: An environment variable specifying the full path to the
//     config file.
//  3. ENV_LEASE_NAME: An environment variable specifying the name of the config
//     file, which is then looked for in the current directory.
//  4. Default: "env-lease.toml" in the current directory.
//
// It also handles path expansion (e.g., expanding ~ to the user's home
// directory). The function returns the resolved absolute path to the
// configuration file or an error if the path cannot be resolved.
func ResolveConfigFile(configFlag string) (string, error) {
	// 1. --config flag
	if configFlag != "" && configFlag != "env-lease.toml" {
		return expandAndAbs(configFlag)
	}

	// 2. ENV_LEASE_CONFIG
	if envConfig := os.Getenv("ENV_LEASE_CONFIG"); envConfig != "" {
		return expandAndAbs(envConfig)
	}

	// 3. ENV_LEASE_NAME
	if envName := os.Getenv("ENV_LEASE_NAME"); envName != "" {
		return expandAndAbs(envName)
	}

	// 4. Default
	return expandAndAbs("env-lease.toml")
}

func expandAndAbs(path string) (string, error) {
	expanded, err := fileutil.ExpandPath(path)
	if err != nil {
		return "", fmt.Errorf("could not expand path '%s': %w", path, err)
	}
	abs, err := filepath.Abs(expanded)
	if err != nil {
		return "", fmt.Errorf("could not get absolute path for '%s': %w", expanded, err)
	}
	return abs, nil
}
