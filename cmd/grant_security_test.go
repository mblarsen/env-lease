package cmd

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/spf13/cobra"
)

func runGrantCommand(t *testing.T, args ...string) (string, string, error) {
	t.Helper()
	oldStdout := os.Stdout
	oldStderr := os.Stderr
	rOut, wOut, _ := os.Pipe()
	rErr, wErr, _ := os.Pipe()
	os.Stdout = wOut
	os.Stderr = wErr

	rootCmd := &cobra.Command{Use: "env-lease"}
	rootCmd.AddCommand(grantCmd)
	rootCmd.AddCommand(revokeCmd)

	os.Setenv("ENV_LEASE_TEST", "1")
	defer os.Unsetenv("ENV_LEASE_TEST")

	rootCmd.SetArgs(append([]string{"grant"}, args...))
	err := rootCmd.Execute()

	wOut.Close()
	wErr.Close()

	var stdout strings.Builder
	var stderr strings.Builder
	io.Copy(&stdout, rOut)
	io.Copy(&stderr, rErr)

	os.Stdout = oldStdout
	os.Stderr = oldStderr

	return stdout.String(), stderr.String(), err
}

func runRevokeCommand(t *testing.T, args ...string) (string, string, error) {
	t.Helper()
	oldStdout := os.Stdout
	oldStderr := os.Stderr
	rOut, wOut, _ := os.Pipe()
	rErr, wErr, _ := os.Pipe()
	os.Stdout = wOut
	os.Stderr = wErr

	rootCmd := &cobra.Command{Use: "env-lease"}
	rootCmd.AddCommand(revokeCmd)

	os.Setenv("ENV_LEASE_TEST", "1")
	defer os.Unsetenv("ENV_LEASE_TEST")

	rootCmd.SetArgs(append([]string{"revoke"}, args...))
	err := rootCmd.Execute()

	wOut.Close()
	wErr.Close()

	var stdout strings.Builder
	var stderr strings.Builder
	io.Copy(&stdout, rOut)
	io.Copy(&stderr, rErr)

	os.Stdout = oldStdout
	os.Stderr = oldStderr

	return stdout.String(), stderr.String(), err
}

func TestGrantSecurity(t *testing.T) {
	// Re-enable this test
	// t.Skip("Skipping security test temporarily to focus on interactive grant refactoring")
	tmpDir := t.TempDir()
	projectRoot := filepath.Join(tmpDir, "project")
	outsideDir := filepath.Join(tmpDir, "outside")
	os.Mkdir(projectRoot, 0755)
	os.Mkdir(outsideDir, 0755)

	configFile := filepath.Join(projectRoot, "env-lease.toml")
	destinationOutside := filepath.Join(outsideDir, "secret")
	destinationInside := filepath.Join(projectRoot, "secret")

	// Test case 1: Destination outside root, no override flag (should fail)
	configContent := fmt.Sprintf(`
[[lease]]
lease_type = "file"
source = "op://vault/item/field"
destination = "%s"
duration = "1h"
`, destinationOutside)
	os.WriteFile(configFile, []byte(configContent), 0644)

	_, _, err := runGrantCommand(t, "--config", configFile, "--interactive=false")
	if err == nil {
		t.Error("Expected an error, but got none")
	}
	expectedError := fmt.Sprintf("destination path '%s' is outside the project root", destinationOutside)
	if !strings.Contains(err.Error(), expectedError) {
		t.Errorf("Expected error message to contain '%s', but it was: %s", expectedError, err.Error())
	}

	// Test case 2: Destination outside root, with override flag (should succeed)
	_, stderr, err := runGrantCommand(t, "--config", configFile, "--destination-outside-root", "--interactive=false")
	if err != nil {
		t.Errorf("Did not expect an error, but got: %v\nStderr: %s", err, stderr)
	}

	// Test case 3: Destination inside root (should succeed)
	configContent = fmt.Sprintf(`
[[lease]]
lease_type = "file"
source = "op://vault/item/field"
destination = "%s"
duration = "1h"
`, destinationInside)
	os.WriteFile(configFile, []byte(configContent), 0644)

	_, stderr, err = runGrantCommand(t, "--config", configFile, "--interactive=false")
	if err != nil {
		t.Errorf("Did not expect an error, but got: %v\nStderr: %s", err, stderr)
	}

	// Test case 4: Exploded lease, check for empty unset in revoke
	configContent = `
[[lease]]
lease_type = "shell"
source = "mock-explode"
duration = "1h"
transform = ["json", "explode"]
`
	os.WriteFile(configFile, []byte(configContent), 0644)
	_, _, err = runGrantCommand(t, "--config", configFile, "--interactive=false")
	if err != nil {
		t.Errorf("Did not expect an error, but got: %v", err)
	}
	stdout, _, err := runRevokeCommand(t, "--all")
	if err != nil {
		t.Errorf("Did not expect an error, but got: %v", err)
	}
	if strings.Contains(stdout, "unset \n") || strings.HasSuffix(stdout, "unset ") {
		t.Errorf("Found empty unset in revoke output: %q", stdout)
	}
}
