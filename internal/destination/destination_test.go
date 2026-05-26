package destination

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/mblarsen/env-lease/internal/lease"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestMaterializerMaterializeEnvLease(t *testing.T) {
	root := t.TempDir()
	dest := filepath.Join(root, ".env")
	l := lease.Lease{
		Source:      "op://vault/item/env",
		Destination: dest,
		LeaseType:   lease.TypeEnv,
		Variable:    "API_KEY",
		Duration:    "1h",
		Format:      "%s=%q",
	}

	result, err := Materializer{ProjectRoot: root, ConfigFile: filepath.Join(root, "env-lease.toml")}.Materialize(l, "secret")
	require.NoError(t, err)
	assert.Equal(t, []string{"Created file: " + dest}, result.Notices)
	require.Len(t, result.Leases, 1)
	assert.Equal(t, dest, result.Leases[0].Destination)

	content, err := os.ReadFile(dest)
	require.NoError(t, err)
	assert.Equal(t, "API_KEY=\"secret\"\n", string(content))
}

func TestMaterializerRejectsExistingEnvValueWithoutOverride(t *testing.T) {
	root := t.TempDir()
	dest := filepath.Join(root, ".env")
	require.NoError(t, os.WriteFile(dest, []byte("API_KEY=\"existing\"\n"), 0600))
	l := lease.Lease{
		Source:      "op://vault/item/env",
		Destination: dest,
		LeaseType:   lease.TypeEnv,
		Variable:    "API_KEY",
		Duration:    "1h",
		Format:      "%s=%q",
	}

	_, err := Materializer{ProjectRoot: root}.Materialize(l, "secret")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "variable 'API_KEY' already has a value")
}

func TestMaterializerMaterializeShellLease(t *testing.T) {
	root := t.TempDir()
	l := lease.Lease{
		Source:      "op://vault/item/shell",
		Destination: filepath.Join(root, "<shell>"),
		LeaseType:   lease.TypeShell,
		Variable:    "SHELL_KEY",
		Duration:    "1h",
	}

	result, err := Materializer{ProjectRoot: root}.Materialize(l, "secret")
	require.NoError(t, err)
	assert.Equal(t, []string{"export SHELL_KEY=\"secret\""}, result.ShellCommands)
	require.Len(t, result.Leases, 1)
}

func TestMaterializerRejectsFileLeaseOutsideRoot(t *testing.T) {
	root := t.TempDir()
	outside := filepath.Join(t.TempDir(), "secret.txt")
	l := lease.Lease{
		Source:         "op://vault/item/file",
		RawDestination: outside,
		Destination:    outside,
		LeaseType:      lease.TypeFile,
		Duration:       "1h",
	}

	_, err := Materializer{ProjectRoot: root}.Materialize(l, "secret")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "outside the project root")
}

func TestRevokerRevokeDestinations(t *testing.T) {
	t.Run("file lease", func(t *testing.T) {
		root := t.TempDir()
		path := filepath.Join(root, "secret.txt")
		require.NoError(t, os.WriteFile(path, []byte("secret"), 0600))

		_, err := (Revoker{}).Revoke(&lease.Lease{LeaseType: lease.TypeFile, Destination: path})
		require.NoError(t, err)
		_, err = os.Stat(path)
		assert.True(t, os.IsNotExist(err))
	})

	t.Run("env lease", func(t *testing.T) {
		root := t.TempDir()
		path := filepath.Join(root, ".envrc")
		require.NoError(t, os.WriteFile(path, []byte("export API_KEY=\"secret\"\nOTHER=\"value\"\n"), 0600))

		_, err := (Revoker{}).Revoke(&lease.Lease{LeaseType: lease.TypeEnv, Destination: path, Variable: "API_KEY"})
		require.NoError(t, err)
		content, err := os.ReadFile(path)
		require.NoError(t, err)
		assert.True(t, strings.Contains(string(content), "export API_KEY=\n"))
		assert.Contains(t, string(content), "OTHER=\"value\"")
	})

	t.Run("shell lease", func(t *testing.T) {
		result, err := (Revoker{}).Revoke(&lease.Lease{LeaseType: lease.TypeShell, Variable: "SHELL_KEY"})
		require.NoError(t, err)
		assert.Equal(t, []string{"unset SHELL_KEY"}, result.ShellCommands)
	})
}
