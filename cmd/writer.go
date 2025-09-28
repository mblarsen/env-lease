package cmd

import (
	"crypto/rand"
	"fmt"
	"github.com/mblarsen/env-lease/internal/config"
	"github.com/mblarsen/env-lease/internal/fileutil"
	"os"
	"strconv"
	"strings"
	"unsafe"
)

func writeLease(l config.Lease, secretVal string, override bool) (bool, error) {
	if _, err := os.Stat(l.Destination); os.IsNotExist(err) {
		// File doesn't exist, so it will be created.
	}

	switch l.LeaseType {
	case "env":
		return writeEnvFile(l.Destination, l.Variable, secretVal, l.Format, override, l.FileMode)
	default:
		return false, fmt.Errorf("unknown lease type: %s", l.LeaseType)
	}
}

func writeEnvFile(path, key, value, format string, override bool, fileModeStr string) (bool, error) {
	fileMode, err := parseFileMode(fileModeStr, 0600)
	if err != nil {
		return false, err
	}

	if _, err := os.Stat(path); os.IsNotExist(err) {
		content := fmt.Sprintf(format+"\n", key, value)
		_, err := fileutil.AtomicWriteFile(path, []byte(content), fileMode)
		return true, err
	}

	content, err := os.ReadFile(path)
	if err != nil {
		return false, fmt.Errorf("failed to read existing file: %w", err)
	}

	lines := strings.Split(string(content), "\n")
	keyExists := false
	// To check for the key, we need to know what the line starts with.
	// The format string tells us. e.g., "export %s=%q" means the line starts with "export KEY=".
	// We can find this by cutting the format string at the first '%'.
	prefix := strings.Split(format, "%")[0] + key + "="
	for i, line := range lines {
		if strings.HasPrefix(line, prefix) {
			keyExists = true
			if override {
				lines[i] = fmt.Sprintf(format, key, value)
			}
			break
		}
	}

	if !keyExists {
		if len(lines) > 0 && lines[len(lines)-1] != "" {
			lines = append(lines, "")
		}
		lines = append(lines, fmt.Sprintf(format, key, value))
	}

	output := strings.Join(lines, "\n")
	if !strings.HasSuffix(output, "\n") {
		output += "\n"
	}
	_, err = fileutil.AtomicWriteFile(path, []byte(output), fileMode)
	return false, err
}

func parseFileMode(fileModeStr string, defaultMode os.FileMode) (os.FileMode, error) {
	if fileModeStr == "" {
		return defaultMode, nil
	}
	mode, err := strconv.ParseUint(fileModeStr, 8, 32)
	if err != nil {
		return 0, fmt.Errorf("invalid file mode: %w", err)
	}
	return os.FileMode(mode), nil
}

// clear overwrites the byte slice with random data to reduce the chance of the
// secret remaining in memory.
func clear(s []byte) {
	_, err := rand.Read(s)
	if err != nil {
		// Fallback to overwriting with zeros if reading random data fails.
		// This should be rare.
		for i := range s {
			s[i] = 0
		}
	}
}

// clearString is a convenience wrapper for clear that works with strings.
func clearString(s string) {
	// This is a bit of a hack to get a mutable byte slice from a string.
	// It's not ideal, but it's the most direct way to clear the string's
	// underlying data without major refactoring.
	b := unsafe.StringData(s)
	clear(unsafe.Slice(b, len(s)))
}