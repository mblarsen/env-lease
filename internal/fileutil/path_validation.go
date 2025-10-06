package fileutil

import (
	"fmt"
	"path/filepath"
	"strings"
)

// IsPathInsideRoot checks if the given path is within the root directory.
// Both paths are expected to be absolute paths.
func IsPathInsideRoot(root, path string) (bool, error) {
	absRoot, err := filepath.Abs(root)
	if err != nil {
		return false, fmt.Errorf("could not get absolute path for root '%s': %w", root, err)
	}

	absPath, err := filepath.Abs(path)
	if err != nil {
		return false, fmt.Errorf("could not get absolute path for path '%s': %w", path, err)
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
