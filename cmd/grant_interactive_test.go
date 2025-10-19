package cmd

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestGrantRunE_Interactive(t *testing.T) {
	tempDir, err := os.MkdirTemp("", "test")
	assert.NoError(t, err)
	defer os.RemoveAll(tempDir)
	os.Setenv("ENV_LEASE_TEST", "1")

	// Helper to create a dummy config file
	writeConfig := func(content string) string {
		path := filepath.Join(tempDir, "env-lease.toml")
		err := os.WriteFile(path, []byte(content), 0644)
		assert.NoError(t, err)
		return path
	}

	t.Run("interactive mode yes", func(t *testing.T) {
		destFile := filepath.Join(tempDir, ".env.interactive.yes")
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

		// Monkeypatch the confirm function to simulate user input
		originalConfirm := confirm
		defer func() { confirm = originalConfirm }()
		resetConfirmState()
		confirm = func(prompt string) bool {
			return true
		}

		err := grantCmd.RunE(grantCmd, []string{})
		assert.NoError(t, err, "grant command failed")

		content, err := os.ReadFile(destFile)
		assert.NoError(t, err, "failed to read destination file")
		expected := `export API_KEY="secret-for-mock"`
		assert.Contains(t, string(content), expected, "file content mismatch")
	})

	t.Run("interactive mode no", func(t *testing.T) {
		destFile := filepath.Join(tempDir, ".env.interactive.no")
		configContent := `
[[lease]]
source = "mock"
destination = "` + destFile + `"
variable = "API_KEY"
duration = "1m"
`
		configFile := writeConfig(configContent)
		grantCmd.Flags().Set("config", configFile)
		grantCmd.Flags().Set("interactive", "true")

		// Monkeypatch the confirm function to simulate user input
		originalConfirm := confirm
		defer func() { confirm = originalConfirm }()
		resetConfirmState()
		confirm = func(prompt string) bool {
			return false
		}

		err := grantCmd.RunE(grantCmd, []string{})
		assert.NoError(t, err, "grant command failed")

		_, err = os.Stat(destFile)
		assert.True(t, os.IsNotExist(err), "destination file should not be created")
	})

}
