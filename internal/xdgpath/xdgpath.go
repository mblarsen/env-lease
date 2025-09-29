package xdgpath

import (
	"fmt"
	"os"
	"path/filepath"
)

func getStateHome() (string, error) {
	if stateHome := os.Getenv("XDG_STATE_HOME"); stateHome != "" {
		return stateHome, nil
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("could not get user home directory: %w", err)
	}
	return filepath.Join(home, ".local", "state"), nil
}

func getRuntimeDir() (string, error) {
	if runtimeDir := os.Getenv("XDG_RUNTIME_DIR"); runtimeDir != "" {
		return runtimeDir, nil
	}
	// Fallback to state home if runtime dir is not available.
	return getStateHome()
}

// StatePath returns the path for a state file, creating the directory if needed.
func StatePath(elem ...string) (string, error) {
	base, err := getStateHome()
	if err != nil {
		return "", err
	}
	dir := filepath.Join(base, "env-lease")
	if err := os.MkdirAll(dir, 0700); err != nil {
		return "", err
	}
	return filepath.Join(append([]string{dir}, elem...)...), nil
}

// RuntimePath returns the path for a runtime file, creating the directory if needed.
func RuntimePath(elem ...string) (string, error) {
	base, err := getRuntimeDir()
	if err != nil {
		return "", err
	}
	dir := filepath.Join(base, "env-lease")
	if err := os.MkdirAll(dir, 0700); err != nil {
		return "", err
	}
	return filepath.Join(append([]string{dir}, elem...)...), nil
}
