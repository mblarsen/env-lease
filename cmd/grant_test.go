package cmd

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestGrantRunE(t *testing.T) {
	tempDir := t.TempDir()
	os.Setenv("ENV_LEASE_TEST", "1")

	// Helper to create a dummy config file
	writeConfig := func(content string) string {
		path := filepath.Join(tempDir, "env-lease.toml")
		err := os.WriteFile(path, []byte(content), 0644)
		if err != nil {
			t.Fatalf("failed to write config: %v", err)
		}
		return path
	}

	t.Run("default format for .envrc", func(t *testing.T) {
		destFile := filepath.Join(tempDir, ".envrc")
		configContent := `
[[lease]]
source = "mock"
destination = "` + destFile + `"
variable = "API_KEY"
duration = "1m"
`
		configFile := writeConfig(configContent)
		grantCmd.Flags().Set("config", configFile)
		err := grantCmd.RunE(grantCmd, []string{})
		if err != nil {
			t.Fatalf("grant command failed: %v", err)
		}

		content, _ := os.ReadFile(destFile)
		expected := `export API_KEY="secret-for-mock"`
		if !strings.Contains(string(content), expected) {
			t.Fatalf("expected content %q, got %q", expected, string(content))
		}
	})

	t.Run("default format for .env", func(t *testing.T) {
		destFile := filepath.Join(tempDir, ".env")
		configContent := `
[[lease]]
source = "mock"
destination = "` + destFile + `"
variable = "API_KEY"
duration = "1m"
`
		configFile := writeConfig(configContent)
		grantCmd.Flags().Set("config", configFile)
		err := grantCmd.RunE(grantCmd, []string{})
		if err != nil {
			t.Fatalf("grant command failed: %v", err)
		}

		content, _ := os.ReadFile(destFile)
		expected := `API_KEY="secret-for-mock"`
		if !strings.Contains(string(content), expected) {
			t.Fatalf("expected content %q, got %q", expected, string(content))
		}
	})

	t.Run("custom format", func(t *testing.T) {
		destFile := filepath.Join(tempDir, "custom.txt")
		configContent := `
[[lease]]
source = "mock"
destination = "` + destFile + `"
variable = "API_KEY"
duration = "1m"
format = "custom_format %s=%s"
`
		configFile := writeConfig(configContent)
		grantCmd.Flags().Set("config", configFile)
		err := grantCmd.RunE(grantCmd, []string{})
		if err != nil {
			t.Fatalf("grant command failed: %v", err)
		}

		content, _ := os.ReadFile(destFile)
		expected := `custom_format API_KEY=secret-for-mock`
		if !strings.Contains(string(content), expected) {
			t.Fatalf("expected content %q, got %q", expected, string(content))
		}
	})

	t.Run("missing format for other file types", func(t *testing.T) {
		destFile := filepath.Join(tempDir, "other.txt")
		configContent := `
[[lease]]
source = "mock"
destination = "` + destFile + `"
variable = "API_key"
duration = "1m"
`
		configFile := writeConfig(configContent)
		grantCmd.Flags().Set("config", configFile)
		err := grantCmd.RunE(grantCmd, []string{})
		if err == nil {
			t.Fatal("expected an error for missing format, but got none")
		}
		expectedErr := "Failed to grant lease:"
		if !strings.Contains(err.Error(), expectedErr) {
			t.Fatalf("expected error to contain %q, got %q", expectedErr, err.Error())
		}
		expectedLease := "Lease: mock"
		if !strings.Contains(err.Error(), expectedLease) {
			t.Fatalf("expected error to contain %q, got %q", expectedLease, err.Error())
		}
		expectedLeaseError := "└─ Error: lease for '" + destFile + "' has no format specified"
		if !strings.Contains(err.Error(), expectedLeaseError) {
			t.Fatalf("expected error to contain %q, got %q", expectedLeaseError, err.Error())
		}
	})

	t.Run("override behavior", func(t *testing.T) {
		destFile := filepath.Join(tempDir, ".env.override")
		// Create a file with an existing value
		initialContent := `API_KEY_OVERRIDE="existing-value"`
		err := os.WriteFile(destFile, []byte(initialContent), 0644)
		if err != nil {
			t.Fatalf("failed to write initial file: %v", err)
		}

		configContent := `
[[lease]]
source = "mock"
destination = "` + destFile + `"
variable = "API_KEY_OVERRIDE"
duration = "1m"
format = "%s=%q"
`
		configFile := writeConfig(configContent)

		// 1. Test without override - should fail
		grantCmd.Flags().Set("config", configFile)
		grantCmd.Flags().Set("override", "false")
		err = grantCmd.RunE(grantCmd, []string{})
		if err == nil {
			t.Fatal("expected an error, but got none")
		}
		expectedErr := "Failed to grant lease:"
		if !strings.Contains(err.Error(), expectedErr) {
			t.Fatalf("expected error to contain %q, got %q", expectedErr, err.Error())
		}
		expectedLeaseError := "└─ Error: failed to write lease: variable 'API_KEY_OVERRIDE' already has a value; use --override to replace it"
		if !strings.Contains(err.Error(), expectedLeaseError) {
			t.Fatalf("expected error to contain %q, got %q", expectedLeaseError, err.Error())
		}

		// 2. Test with override - should succeed
		grantCmd.Flags().Set("override", "true")
		err = grantCmd.RunE(grantCmd, []string{})
		if err != nil {
			t.Fatalf("grant command failed with override flag: %v", err)
		}

		content, _ := os.ReadFile(destFile)
		expected := `API_KEY_OVERRIDE="secret-for-mock"`
		if !strings.Contains(string(content), expected) {
			t.Fatalf("expected content %q, got %q", expected, string(content))
		}
	})

	t.Run("continue on error behavior", func(t *testing.T) {
		destFile := filepath.Join(tempDir, ".env.continue")
		configContent := `
[[lease]]
source = "mock-fail"
destination = "` + destFile + `"
variable = "FAIL_KEY"
duration = "1m"
format = "%s=%q"

[[lease]]
source = "mock"
destination = "` + destFile + `"
variable = "SUCCESS_KEY"
duration = "1m"
format = "%s=%q"
`
		configFile := writeConfig(configContent)

		// 1. Test without flag - should fail fast, but with new format
		grantCmd.Flags().Set("config", configFile)
		grantCmd.Flags().Set("continue-on-error", "false")
		err := grantCmd.RunE(grantCmd, []string{})
		if err == nil {
			t.Fatal("expected an error, but got none")
		}
		expectedErr := "Failed to grant lease:"
		if !strings.Contains(err.Error(), expectedErr) {
			t.Fatalf("expected error to contain %q, got %q", expectedErr, err.Error())
		}
		expectedLease := "Lease: mock-fail"
		if !strings.Contains(err.Error(), expectedLease) {
			t.Fatalf("expected error to contain %q, got %q", expectedLease, err.Error())
		}
		expectedLeaseError := "└─ Error: failed to fetch mock secret"
		if !strings.Contains(err.Error(), expectedLeaseError) {
			t.Fatalf("expected error to contain %q, got %q", expectedLeaseError, err.Error())
		}

		// 2. Test with flag - should continue and aggregate errors
		grantCmd.Flags().Set("continue-on-error", "true")
		err = grantCmd.RunE(grantCmd, []string{})
		if err == nil {
			t.Fatal("expected an error, but got none")
		}
		expectedErr = "Failed to grant lease:"
		if !strings.Contains(err.Error(), expectedErr) {
			t.Fatalf("expected aggregated error to contain %q, got %q", expectedErr, err.Error())
		}
		if !strings.Contains(err.Error(), expectedLease) {
			t.Fatalf("expected aggregated error to contain %q, got %q", expectedLease, err.Error())
		}
		if !strings.Contains(err.Error(), expectedLeaseError) {
			t.Fatalf("expected aggregated error to contain %q, got %q", expectedLeaseError, err.Error())
		}

		// Check that the successful lease was still written
		content, _ := os.ReadFile(destFile)
		expectedContent := `SUCCESS_KEY="secret-for-mock"`
		if !strings.Contains(string(content), expectedContent) {
			t.Fatalf("expected content %q, got %q", expectedContent, string(content))
		}
	})
}
