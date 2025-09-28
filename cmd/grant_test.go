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
variable = "API_KEY"
duration = "1m"
`
		configFile := writeConfig(configContent)
		grantCmd.Flags().Set("config", configFile)
		err := grantCmd.RunE(grantCmd, []string{})
		if err == nil {
			t.Fatal("expected an error for missing format, but got none")
		}
		expectedErr := "lease for '" + destFile + "' has no format specified"
		if err.Error() != expectedErr {
			t.Fatalf("expected error %q, got %q", expectedErr, err.Error())
		}
	})
}
