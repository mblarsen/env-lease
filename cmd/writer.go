package cmd

import (
	"crypto/rand"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"unsafe"

	"github.com/mblarsen/env-lease/internal/fileutil"
	"github.com/mblarsen/env-lease/internal/lease"
)

func writeLease(l lease.Lease, secretVal, projectRoot string, override bool) (bool, error) {
	dest := l.Destination
	if !filepath.IsAbs(dest) {
		dest = filepath.Join(projectRoot, dest)
	} else {
		dest = filepath.Clean(dest)
	}

	if _, err := os.Stat(dest); os.IsNotExist(err) {
		// File doesn't exist, so it will be created.
	}

	fileMode, err := l.ParseFileMode(0600)
	if err != nil {
		return false, err
	}

	switch l.LeaseType {
	case "env":
		return writeEnvFile(dest, l.Variable, secretVal, l.Format, override, fileMode)
	case "file":
		return writeFile(dest, secretVal, fileMode)
	case "shell":
		return false, fmt.Errorf("the 'shell' lease type should not be handled by writeLease")
	default:
		return false, fmt.Errorf("unknown lease type: %s", l.LeaseType)
	}
}

func writeFile(path, value string, fileMode os.FileMode) (bool, error) {
	if _, err := os.Stat(path); os.IsNotExist(err) {
		_, err := fileutil.AtomicWriteFile(path, []byte(value), fileMode)
		return true, err
	}

	// If the file exists, we just overwrite it.
	_, err := fileutil.AtomicWriteFile(path, []byte(value), fileMode)
	return false, err
}

func writeEnvFile(path, key, value, format string, override bool, fileMode os.FileMode) (bool, error) {
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
	prefix := strings.Split(format, "%")[0] + key + "="
	for i, line := range lines {
		if strings.HasPrefix(line, prefix) {
			keyExists = true
			newLine := fmt.Sprintf(format, key, value)
			if line == newLine {
				return false, nil
			}

			// Check if the line has a value.
			parts := strings.SplitN(line, "=", 2)
			hasValue := len(parts) > 1 && strings.Trim(parts[1], `""`) != ""

			if hasValue && !override {
				return false, fmt.Errorf("variable '%s' already has a value; use --override to replace it", key)
			}

			lines[i] = newLine
			break
		}
	}

	if !keyExists {
		lines = append(lines, fmt.Sprintf(format, key, value))
	}

	var nonEmptyLines []string
	for _, line := range lines {
		if strings.TrimSpace(line) != "" {
			nonEmptyLines = append(nonEmptyLines, line)
		}
	}

	output := strings.Join(nonEmptyLines, "\n") + "\n"
	_, err = fileutil.AtomicWriteFile(path, []byte(output), fileMode)
	return false, err
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
