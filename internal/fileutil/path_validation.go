package fileutil

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// ExpandPath expands the tilde prefix `~/` to the user's home directory.
func ExpandPath(path string) (string, error) {
	if !strings.HasPrefix(path, "~/") {
		return path, nil
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("could not get user home directory: %w", err)
	}
	return filepath.Join(home, path[2:]), nil
}

// IsPathInsideRoot checks if the given path is within the root directory.
// Both paths are expected to be absolute paths.
func IsPathInsideRoot(root, path string) (bool, error) {
	if !filepath.IsAbs(path) {
		path = filepath.Join(root, path)
	}

	expandedPath, err := ExpandPath(path)
	if err != nil {
		return false, fmt.Errorf("could not expand path '%s': %w", path, err)
	}

	absRoot, err := filepath.Abs(root)
	if err != nil {
		return false, fmt.Errorf("could not get absolute path for root '%s': %w", root, err)
	}

	absPath, err := filepath.Abs(expandedPath)
	if err != nil {
		return false, fmt.Errorf("could not get absolute path for path '%s': %w", expandedPath, err)
	}

	// On Windows, drive letters must match.
	if filepath.VolumeName(absRoot) != filepath.VolumeName(absPath) {
		return false, nil
	}

	rel, err := filepath.Rel(absRoot, absPath)
	if err != nil {
		return false, fmt.Errorf("could not get relative path: %w", err)
	}

	// If the relative path starts with '..', it's outside the root.
	// This also handles the case where the paths are the same (rel is '.').
	return !strings.HasPrefix(rel, ".."), nil
}
