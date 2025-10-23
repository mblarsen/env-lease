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

	t.Run("interactive mode multi-phase flow", func(t *testing.T) {
		destFileSimple := filepath.Join(tempDir, ".env.simple")
		destFileExplode := filepath.Join(tempDir, ".env.explode")
		configContent := `
[[lease]]
source = "mock-simple"
destination = "` + destFileSimple + `"
variable = "SIMPLE_KEY"
duration = "1m"
format = "%s=%q"

[[lease]]
source = "mock-explode"
destination = "` + destFileExplode + `"
duration = "1m"
transform = ["json", "explode"]
format = "%s=%q"
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
			return true // Always say yes for this test
		}

		err := grantCmd.RunE(grantCmd, []string{})
		assert.NoError(t, err, "grant command failed")

		// --- Assertions ---
		// This is the core of the test. It validates the strict ordering of the prompts.
		// With the refactored implementation, the first two prompts must be the top-level
		// sources (Round 1), and the next two must be the sub-leases (Round 2).
		// The current (incorrect) implementation will fail this test because it will
		// produce an interleaved order like: SIMPLE_KEY, mock-explode, key1, key2.
		// expectedPromptOrder := []string{
		// 	"Grant 'SIMPLE_KEY'?",
		// 	"Grant leases from 'mock-explode' (json, explode)?",
		// 	"Grant lease for 'key1'?",
		// 	"Grant lease for 'key2'?",
		// }

		// To make the test robust against map iteration randomness in the sub-leases,
		// we will check the Round 1 prompts explicitly and then check for the presence
		// of Round 2 prompts.
		assert.Equal(t, "Grant 'SIMPLE_KEY'?", prompts[0], "First prompt should be for the simple lease.")
		assert.Equal(t, "Grant leases from 'mock-explode' (json, explode)?", prompts[1], "Second prompt should be for the explode source.")

		// Check that the sub-lease prompts for Round 2 came last.
		round2Prompts := prompts[2:]
		assert.Contains(t, round2Prompts, "Grant lease for 'KEY1'?", "Round 2 prompts should contain KEY1")
		assert.Contains(t, round2Prompts, "Grant lease for 'KEY2'?", "Round 2 prompts should contain KEY2")

		// Also verify that the files were written correctly
		simpleContent, err := os.ReadFile(destFileSimple)
		assert.NoError(t, err)
		assert.Contains(t, string(simpleContent), `SIMPLE_KEY="secret-for-mock-simple"`)

		explodeContent, err := os.ReadFile(destFileExplode)
		assert.NoError(t, err)
		assert.Contains(t, string(explodeContent), `KEY1="VALUE1"`)
		assert.Contains(t, string(explodeContent), `KEY2="VALUE2"`)
	})
}
