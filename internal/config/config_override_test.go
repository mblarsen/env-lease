package config

import (
	"fmt"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestLocalOverride(t *testing.T) {
	// Create a temporary directory for the test configs
	dir, err := os.MkdirTemp("", "config-override-test")
	assert.NoError(t, err)
	defer os.RemoveAll(dir)

	// Main config content
	mainConfigContent := `
[[lease]]
source = "op://vault/item/secret"
destination = ".env"
variable = "API_KEY"
duration = "1h"
`

	// Local override config content
	localConfigContent := `
[[lease]]
source = "op://vault/item/another-secret"
destination = ".env.local"
variable = "LOCAL_API_KEY"
duration = "30m"
`

	// Write the main and local config files
	mainConfigPath := filepath.Join(dir, "env-lease.toml")
	err = os.WriteFile(mainConfigPath, []byte(mainConfigContent), 0644)
	assert.NoError(t, err)

	localConfigPath := filepath.Join(dir, "env-lease.local.toml")
	err = os.WriteFile(localConfigPath, []byte(localConfigContent), 0644)
	assert.NoError(t, err)

	// Load the config
	config, err := Load(mainConfigPath, "")
	fmt.Println(config, err)
	assert.NoError(t, err)
	assert.NotNil(t, config)

	// Verify that the leases from both files are loaded
	assert.Len(t, config.Lease, 2)

	// Check the first lease (from main config)
	assert.Equal(t, "op://vault/item/secret", config.Lease[0].Source)
	assert.Equal(t, ".env", config.Lease[0].Destination)
	assert.Equal(t, "API_KEY", config.Lease[0].Variable)
	assert.Equal(t, "1h", config.Lease[0].Duration)

	// Check the second lease (from local config)
	assert.Equal(t, "op://vault/item/another-secret", config.Lease[1].Source)
	assert.Equal(t, ".env.local", config.Lease[1].Destination)
	assert.Equal(t, "LOCAL_API_KEY", config.Lease[1].Variable)
	assert.Equal(t, "30m", config.Lease[1].Duration)
}

func TestConfigFlagOverride(t *testing.T) {
	// Create a temporary directory for the test configs
	dir, err := os.MkdirTemp("", "config-flag-override-test")
	assert.NoError(t, err)
	defer os.RemoveAll(dir)

	// Main config content
	mainConfigContent := `
[[lease]]
source = "op://vault/item/secret"
destination = ".env"
variable = "API_KEY"
duration = "1h"
`

	// Local override config content
	localConfigContent := `
[[lease]]
source = "op://vault/item/another-secret"
destination = ".env.local"
variable = "LOCAL_API_KEY"
duration = "30m"
`

	// Write the main and local config files
	mainConfigPath := filepath.Join(dir, "custom.toml")
	err = os.WriteFile(mainConfigPath, []byte(mainConfigContent), 0644)
	assert.NoError(t, err)

	localConfigPath := filepath.Join(dir, "custom.local.toml")
	err = os.WriteFile(localConfigPath, []byte(localConfigContent), 0644)
	assert.NoError(t, err)

	// Load the config
	config, err := Load(mainConfigPath, "")
	assert.NoError(t, err)
	assert.NotNil(t, config)

	// Verify that the leases from both files are loaded
	assert.Len(t, config.Lease, 2)
}

func TestEnvLeaseConfigOverride(t *testing.T) {
	// Create a temporary directory for the test configs
	dir, err := os.MkdirTemp("", "env-lease-config-override-test")
	assert.NoError(t, err)
	defer os.RemoveAll(dir)

	// Main config content
	mainConfigContent := `
[[lease]]
source = "op://vault/item/secret"
destination = ".env"
variable = "API_KEY"
duration = "1h"
`

	// Local override config content
	localConfigContent := `
[[lease]]
source = "op://vault/item/another-secret"
destination = ".env.local"
variable = "LOCAL_API_KEY"
duration = "30m"
`

	// Write the main and local config files
	mainConfigPath := filepath.Join(dir, "custom.toml")
	err = os.WriteFile(mainConfigPath, []byte(mainConfigContent), 0644)
	assert.NoError(t, err)

	localConfigPath := filepath.Join(dir, "custom.local.toml")
	err = os.WriteFile(localConfigPath, []byte(localConfigContent), 0644)
	assert.NoError(t, err)

	// Set the environment variable
	t.Setenv("ENV_LEASE_CONFIG", mainConfigPath)

	// Resolve the config file
	resolvedPath, err := ResolveConfigFile("")
	assert.NoError(t, err)

	// Load the config
	config, err := Load(resolvedPath, "")
	assert.NoError(t, err)
	assert.NotNil(t, config)

	// Verify that the leases from both files are loaded
	assert.Len(t, config.Lease, 2)
}

func TestNoLocalFile(t *testing.T) {
	// Create a temporary directory for the test configs
	dir, err := os.MkdirTemp("", "no-local-file-test")
	assert.NoError(t, err)
	defer os.RemoveAll(dir)

	// Main config content
	mainConfigContent := `
[[lease]]
source = "op://vault/item/secret"
destination = ".env"
variable = "API_KEY"
duration = "1h"
`

	// Write the main config file
	mainConfigPath := filepath.Join(dir, "env-lease.toml")
	err = os.WriteFile(mainConfigPath, []byte(mainConfigContent), 0644)
	assert.NoError(t, err)

	// Load the config
	config, err := Load(mainConfigPath, "")
	assert.NoError(t, err)
	assert.NotNil(t, config)

	// Verify that only the lease from the main file is loaded
	assert.Len(t, config.Lease, 1)
}

func TestEmptyLocalFile(t *testing.T) {
	// Create a temporary directory for the test configs
	dir, err := os.MkdirTemp("", "empty-local-file-test")
	assert.NoError(t, err)
	defer os.RemoveAll(dir)

	// Main config content
	mainConfigContent := `
[[lease]]
source = "op://vault/item/secret"
destination = ".env"
variable = "API_KEY"
duration = "1h"
`

	// Write the main and empty local config files
	mainConfigPath := filepath.Join(dir, "env-lease.toml")
	err = os.WriteFile(mainConfigPath, []byte(mainConfigContent), 0644)
	assert.NoError(t, err)

	localConfigPath := filepath.Join(dir, "env-lease.local.toml")
	err = os.WriteFile(localConfigPath, []byte(""), 0644)
	assert.NoError(t, err)

	// Load the config
	config, err := Load(mainConfigPath, "")
	assert.NoError(t, err)
	assert.NotNil(t, config)

	// Verify that only the lease from the main file is loaded
	assert.Len(t, config.Lease, 1)
}

func TestEnvLeaseLocalConfigOverride(t *testing.T) {
	// Create a temporary directory for the test configs
	dir, err := os.MkdirTemp("", "env-lease-local-config-override-test")
	assert.NoError(t, err)
	defer os.RemoveAll(dir)

	// Main config content
	mainConfigContent := `
[[lease]]
source = "op://vault/item/secret"
destination = ".env"
variable = "API_KEY"
duration = "1h"
`

	// Local override config content
	localConfigContent := `
[[lease]]
source = "op://vault/item/another-secret"
destination = ".env.local"
variable = "LOCAL_API_KEY"
duration = "30m"
`

	// Write the main and local config files
	mainConfigPath := filepath.Join(dir, "env-lease.toml")
	err = os.WriteFile(mainConfigPath, []byte(mainConfigContent), 0644)
	assert.NoError(t, err)

	localConfigPath := filepath.Join(dir, "custom.local.toml")
	err = os.WriteFile(localConfigPath, []byte(localConfigContent), 0644)
	assert.NoError(t, err)

	// Set the environment variable
	t.Setenv("ENV_LEASE_LOCAL_CONFIG", localConfigPath)

	// Load the config
	config, err := Load(mainConfigPath, "")
	assert.NoError(t, err)
	assert.NotNil(t, config)

	// Verify that the leases from both files are loaded
	assert.Len(t, config.Lease, 2)
}

func TestEnvLeaseLocalNameOverride(t *testing.T) {
	// Create a temporary directory for the test configs
	dir, err := os.MkdirTemp("", "env-lease-local-name-override-test")
	assert.NoError(t, err)
	defer os.RemoveAll(dir)

	// Main config content
	mainConfigContent := `
[[lease]]
source = "op://vault/item/secret"
destination = ".env"
variable = "API_KEY"
duration = "1h"
`

	// Local override config content
	localConfigContent := `
[[lease]]
source = "op://vault/item/another-secret"
destination = ".env.local"
variable = "LOCAL_API_KEY"
duration = "30m"
`

	// Write the main and local config files
	mainConfigPath := filepath.Join(dir, "env-lease.toml")
	err = os.WriteFile(mainConfigPath, []byte(mainConfigContent), 0644)
	assert.NoError(t, err)

	localConfigPath := filepath.Join(dir, "custom.local.toml")
	err = os.WriteFile(localConfigPath, []byte(localConfigContent), 0644)
	assert.NoError(t, err)

	// Set the environment variable
	t.Setenv("ENV_LEASE_LOCAL_NAME", "custom.local.toml")

	// Load the config
	config, err := Load(mainConfigPath, "")
	assert.NoError(t, err)
	assert.NotNil(t, config)

	// Verify that the leases from both files are loaded
	assert.Len(t, config.Lease, 2)
}

func TestLocalConfigFlagOverride(t *testing.T) {
	// Create a temporary directory for the test configs
	dir, err := os.MkdirTemp("", "local-config-flag-override-test")
	assert.NoError(t, err)
	defer os.RemoveAll(dir)

	// Main config content
	mainConfigContent := `
[[lease]]
source = "op://vault/item/secret"
destination = ".env"
variable = "API_KEY"
duration = "1h"
`

	// Local override config content
	localConfigContent := `
[[lease]]
source = "op://vault/item/another-secret"
destination = ".env.local"
variable = "LOCAL_API_KEY"
duration = "30m"
`

	// Write the main and local config files
	mainConfigPath := filepath.Join(dir, "env-lease.toml")
	err = os.WriteFile(mainConfigPath, []byte(mainConfigContent), 0644)
	assert.NoError(t, err)

	localConfigPath := filepath.Join(dir, "custom.local.toml")
	err = os.WriteFile(localConfigPath, []byte(localConfigContent), 0644)
	assert.NoError(t, err)

	// Load the config
	config, err := Load(mainConfigPath, localConfigPath)
	assert.NoError(t, err)
	assert.NotNil(t, config)

	// Verify that the leases from both files are loaded
	assert.Len(t, config.Lease, 2)
}
