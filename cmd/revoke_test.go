package cmd

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func TestRevokePreflightDaemonNotRunning(t *testing.T) {
	if os.Getenv("GO_WANT_HELPER_PROCESS_REVOKE") == "1" {
		resetConfirmState()
		_ = os.Unsetenv("ENV_LEASE_TEST")

		tempDir := t.TempDir()
		configPath := filepath.Join(tempDir, "env-lease.toml")
		if err := os.WriteFile(configPath, []byte(`
[[lease]]
source = "mock"
destination = "`+filepath.Join(tempDir, ".env")+`"
variable = "API_KEY"
duration = "1m"
`), 0644); err != nil {
			t.Fatalf("failed to write config: %v", err)
		}

		revokeCmd.Flags().Set("config", configPath)
		revokeCmd.Flags().Set("all", "false")
		revokeCmd.Flags().Set("interactive", "false")
		revokeCmd.Flags().Set("no-direnv", "false")

		_ = revokeCmd.RunE(revokeCmd, []string{})
		t.Fatalf("expected revoke command to exit when daemon is unavailable")
		return
	}

	cmd := exec.Command(os.Args[0], "-test.run", "TestRevokePreflightDaemonNotRunning")
	cmd.Env = append(os.Environ(),
		"GO_WANT_HELPER_PROCESS_REVOKE=1",
		"ENV_LEASE_TEST=",
	)

	output, err := cmd.CombinedOutput()
	if err == nil {
		t.Fatal("expected revoke command to fail when daemon is unavailable")
	}
	exitErr, ok := err.(*exec.ExitError)
	if !ok || exitErr.ExitCode() == 0 {
		t.Fatalf("expected non-zero exit code, got %v", err)
	}
	if !strings.Contains(string(output), "Error: env-lease daemon is not running. Please start it with 'env-lease daemon start'.") {
		t.Fatalf("expected daemon offline message, got %q", string(output))
	}
}
