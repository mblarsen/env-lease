package cmd

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
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

	t.Run("interactive mode", func(t *testing.T) {
		destFile := filepath.Join(tempDir, ".env.interactive")
		configContent := `
[[lease]]
source = "mock"
destination = "` + destFile + `"
variable = "API_KEY"
duration = "1m"
format = "export %s=%q"
`
		configFile := writeConfig(configContent)
		grantCmd.Flags().Set("config", configFile)
		grantCmd.Flags().Set("interactive", "true")

		// Test case where user denies
		resetConfirmState()
		confirm = func(prompt string) bool { return false }
		err := grantCmd.RunE(grantCmd, []string{})
		if err != nil {
			t.Fatalf("grant command failed: %v", err)
		}
		content, err := os.ReadFile(destFile)
		if err == nil && len(content) > 0 {
			t.Fatalf("file should be empty, but has content: %s", content)
		}

		// Test case where user accepts
		resetConfirmState()
		confirm = func(prompt string) bool { return true }
		grantCmd.Flags().Set("interactive", "true")
		err = grantCmd.RunE(grantCmd, []string{})
		if err != nil {
			t.Fatalf("grant command failed: %v", err)
		}
		content, err = os.ReadFile(destFile)
		if err != nil {
			t.Fatalf("failed to read destination file: %v", err)
		}
		expected := `export API_KEY="secret-for-mock"`
		if !strings.Contains(string(content), expected) {
			t.Fatalf("expected content %q, got %q", expected, string(content))
		}
	})

	t.Run("interactive mode with explode", func(t *testing.T) {
		destFile := filepath.Join(tempDir, ".env.interactive.explode")
		configContent := `
[[lease]]
source = "mock-explode"
destination = "` + destFile + `"
duration = "1m"
transform = ["json", "explode"]
lease_type = "shell"
`
		configFile := writeConfig(configContent)
		grantCmd.Flags().Set("config", configFile)
		grantCmd.Flags().Set("interactive", "true")

		var prompts []string
		originalConfirm := confirm
		defer func() { confirm = originalConfirm }()

		resetConfirmState()
		confirm = func(prompt string) bool {
			prompts = append(prompts, prompt)
			return prompt == "Grant leases from 'mock-explode' (json, explode)?"
		}

		err := grantCmd.RunE(grantCmd, []string{})
		if err != nil {
			t.Fatalf("grant command failed: %v", err)
		}

		// Prompts are no longer deterministic due to parallel fetching
		// and map iteration. We check that the essential prompts were made.
		shownPrompts := make(map[string]bool)
		for _, p := range prompts {
			shownPrompts[p] = true
		}

		assert.True(t, shownPrompts["Grant leases from 'mock-explode' (json, explode)?"], "did not show parent prompt")
		assert.False(t, shownPrompts["Grant lease for 'key1'?"], "should not show key1 prompt")
		assert.False(t, shownPrompts["Grant lease for 'key2'?"], "should not show key2 prompt")
	})
}
