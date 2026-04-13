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
// Symlinks are resolved for existing path components to enforce true
// filesystem containment instead of lexical containment.
func IsPathInsideRoot(root, path string) (bool, error) {
	expandedPath, err := ExpandPath(path)
	if err != nil {
		return false, fmt.Errorf("could not expand path '%s': %w", path, err)
	}

	absRoot, err := filepath.Abs(root)
	if err != nil {
		return false, fmt.Errorf("could not get absolute path for root '%s': %w", root, err)
	}

	resolvedRoot, err := filepath.EvalSymlinks(absRoot)
	if err != nil {
		return false, fmt.Errorf("could not resolve root path '%s': %w", absRoot, err)
	}

	// Clean the path before symlink resolution so that .. segments are
	// resolved lexically first, matching the behaviour of writeLease which
	// uses filepath.Join (and therefore filepath.Clean) to build the
	// destination. Without this, a path like link/../secret would be
	// resolved through the symlink at "link" and the .. would ascend from
	// the symlink target, producing a false-positive denial even though
	// filepath.Join would have cancelled link/.. and written inside root.
	if !filepath.IsAbs(expandedPath) {
		expandedPath = filepath.Join(absRoot, expandedPath)
	} else {
		expandedPath = filepath.Clean(expandedPath)
	}

	resolvedPath, err := resolvePathAllowMissing(expandedPath)
	if err != nil {
		return false, fmt.Errorf("could not resolve path '%s': %w", expandedPath, err)
	}

	// On Windows, drive letters must match.
	if filepath.VolumeName(resolvedRoot) != filepath.VolumeName(resolvedPath) {
		return false, nil
	}

	rel, err := filepath.Rel(resolvedRoot, resolvedPath)
	if err != nil {
		return false, fmt.Errorf("could not get relative path: %w", err)
	}

	if rel == ".." || strings.HasPrefix(rel, ".."+string(os.PathSeparator)) {
		return false, nil
	}

	return true, nil
}

func resolvePathAllowMissing(path string) (string, error) {
	current := path
	missingParts := make([]string, 0, 4)

	for {
		resolved, err := filepath.EvalSymlinks(current)
		if err == nil {
			for i := len(missingParts) - 1; i >= 0; i-- {
				resolved = filepath.Join(resolved, missingParts[i])
			}
			return resolved, nil
		}

		if !os.IsNotExist(err) {
			return "", err
		}

		parent, component, ok := splitPathPreserve(current)
		if !ok {
			return "", err
		}

		missingParts = append(missingParts, component)
		current = parent
	}
}

func splitPathPreserve(path string) (parent, component string, ok bool) {
	if path == "" {
		return "", "", false
	}

	volume := filepath.VolumeName(path)
	volumeLen := len(volume)
	end := len(path)

	for end > volumeLen+1 && os.IsPathSeparator(path[end-1]) {
		end--
	}

	if end <= volumeLen+1 {
		if len(path) > volumeLen && os.IsPathSeparator(path[volumeLen]) {
			return "", "", false
		}
	}

	idx := end - 1
	for idx >= volumeLen && !os.IsPathSeparator(path[idx]) {
		idx--
	}

	if idx < volumeLen {
		if filepath.IsAbs(path) {
			return volume + string(os.PathSeparator), path[volumeLen:end], true
		}
		return ".", path[:end], true
	}

	component = path[idx+1 : end]
	if idx == volumeLen {
		parent = path[:idx+1]
	} else {
		parent = path[:idx]
	}

	if parent == "" {
		parent = "."
	}

	return parent, component, true
}
