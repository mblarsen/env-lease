package cmd

import (
	"os"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestColorsEnabledRespectsNoColor(t *testing.T) {
	assert.False(t, colorsEnabledWithEnv(os.Stdout, func(key string) (string, bool) {
		if key == "NO_COLOR" {
			return "1", true
		}
		return "", false
	}))
}

func TestColorsEnabledRequiresTerminal(t *testing.T) {
	file, err := os.CreateTemp(t.TempDir(), "output")
	require.NoError(t, err)
	defer file.Close()

	assert.False(t, colorsEnabledWithEnv(file, func(string) (string, bool) {
		return "", false
	}))
}
