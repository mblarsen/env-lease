package cmd

import (
	"bytes"
	"fmt"
	"io/ioutil"
	"os"
	"path/filepath"
	"testing"

	"github.com/spf13/cobra"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestConvertCmd(t *testing.T) {
	// Create a temporary directory for our test files
	tmpDir, err := ioutil.TempDir("", "env-lease-test")
	require.NoError(t, err)
	defer os.RemoveAll(tmpDir)

	// Create a sample .envrc file
	envrcContent := `
# This is a comment
export API_KEY="op://vault/item/field"
SECRET_TOKEN='op://vault/item2/field2'
export   EXTRA_VAR=op://vault/item3/field3
INVALID_LINE
`
	envrcPath := filepath.Join(tmpDir, ".envrc")
	err = ioutil.WriteFile(envrcPath, []byte(envrcContent), 0644)
	require.NoError(t, err)

	// Test case 1: Read from .envrc in current directory
	t.Run("reads from .envrc", func(t *testing.T) {
		// Change to the temporary directory
		originalWd, err := os.Getwd()
		require.NoError(t, err)
		err = os.Chdir(tmpDir)
		require.NoError(t, err)
		defer os.Chdir(originalWd)

		expectedToml := `[[lease]]
variable = "API_KEY"
source = "op://vault/item/field"
destination = ".envrc"
duration = "1h"

[[lease]]
variable = "SECRET_TOKEN"
source = "op://vault/item2/field2"
destination = ".envrc"
duration = "1h"

[[lease]]
variable = "EXTRA_VAR"
source = "op://vault/item3/field3"
destination = ".envrc"
duration = "1h"

`
		// Execute the command
		output, err := executeCommand(rootCmd, "convert")
		require.NoError(t, err)
		assert.Equal(t, expectedToml, output)
	})

	// Test case 2: Read from a specific file
	t.Run("reads from specified file", func(t *testing.T) {
		expectedToml := fmt.Sprintf(`[[lease]]
variable = "API_KEY"
source = "op://vault/item/field"
destination = "%s"
duration = "1h"

[[lease]]
variable = "SECRET_TOKEN"
source = "op://vault/item2/field2"
destination = "%s"
duration = "1h"

[[lease]]
variable = "EXTRA_VAR"
source = "op://vault/item3/field3"
destination = "%s"
duration = "1h"

`, envrcPath, envrcPath, envrcPath)
		// Execute the command with the file path
		output, err := executeCommand(rootCmd, "convert", envrcPath)
		require.NoError(t, err)
		assert.Equal(t, expectedToml, output)
	})

	// Test case 3: No file found
	t.Run("no file found", func(t *testing.T) {
		// Change to a directory with no .env or .envrc
		emptyDir, err := ioutil.TempDir("", "env-lease-empty")
		require.NoError(t, err)
		defer os.RemoveAll(emptyDir)

		originalWd, err := os.Getwd()
		require.NoError(t, err)
		err = os.Chdir(emptyDir)
		require.NoError(t, err)
		defer os.Chdir(originalWd)

		// Execute the command
		_, err = executeCommand(rootCmd, "convert")
		assert.Error(t, err)
	})
}

// executeCommand is a helper function to execute a cobra command and capture its output.
func executeCommand(cmd *cobra.Command, args ...string) (string, error) {
	buf := new(bytes.Buffer)
	cmd.SetOut(buf)
	cmd.SetErr(buf)
	cmd.SetArgs(args)

	err := cmd.Execute()
	return buf.String(), err
}

func TestConvertToToml(t *testing.T) {
	testCases := []struct {
		name     string
		input    string
		expected string
	}{
		{
			name: "simple case",
			input: `export API_KEY="op://vault/item/field"`,
			expected: `[[lease]]
variable = "API_KEY"
source = "op://vault/item/field"
destination = ".envrc"
duration = "1h"

`,
		},
		{
			name: "multiple entries",
			input: `
export API_KEY="op://vault/item/field"
export ANOTHER_KEY="op://vault/item2/field2"
`,
			expected: `[[lease]]
variable = "API_KEY"
source = "op://vault/item/field"
destination = ".envrc"
duration = "1h"

[[lease]]
variable = "ANOTHER_KEY"
source = "op://vault/item2/field2"
destination = ".envrc"
duration = "1h"

`,
		},
		{
			name: "mixed with other lines",
			input: `
# some comment
export API_KEY="op://vault/item/field"
echo "hello"
ANOTHER_KEY='op://vault/item2/field2'
`,
			expected: `[[lease]]
variable = "API_KEY"
source = "op://vault/item/field"
destination = ".envrc"
duration = "1h"

[[lease]]
variable = "ANOTHER_KEY"
source = "op://vault/item2/field2"
destination = ".envrc"
duration = "1h"

`,
		},
		{
			name: "no quotes",
			input: `DB_PASSWORD=op://vault/db/password`,
			expected: `[[lease]]
variable = "DB_PASSWORD"
source = "op://vault/db/password"
destination = ".envrc"
duration = "1h"

`,
		},
		{
			name:  "empty input",
			input: ``,

			expected: "",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			output := convertToToml(tc.input, ".envrc")
			assert.Equal(t, tc.expected, output)
		})
	}
}