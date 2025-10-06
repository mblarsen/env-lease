package fileutil

import (
	"os"
	"path/filepath"
	"testing"
)

func TestIsPathInsideRoot(t *testing.T) {
	// Create temporary directories for testing
	tmpDir := t.TempDir()
	rootDir := filepath.Join(tmpDir, "root")
	insideDir := filepath.Join(rootDir, "inside")
	outsideDir := filepath.Join(tmpDir, "outside")

	if err := os.MkdirAll(insideDir, 0755); err != nil {
		t.Fatalf("Failed to create test directories: %v", err)
	}
	if err := os.MkdirAll(outsideDir, 0755); err != nil {
		t.Fatalf("Failed to create test directories: %v", err)
	}

	testCases := []struct {
		name        string
		root        string
		path        string
		expected    bool
		expectError bool
	}{
		{
			name:     "Path inside root",
			root:     rootDir,
			path:     filepath.Join(rootDir, "some", "path"),
			expected: true,
		},
		{
			name:     "Path is the root",
			root:     rootDir,
			path:     rootDir,
			expected: true,
		},
		{
			name:     "Path outside root",
			root:     rootDir,
			path:     filepath.Join(outsideDir, "some", "path"),
			expected: false,
		},
		{
			name:     "Path is a sibling of root",
			root:     rootDir,
			path:     outsideDir,
			expected: false,
		},
		{
			name:     "Path is a parent of root",
			root:     rootDir,
			path:     tmpDir,
			expected: false,
		},
		{
			name:     "Relative path inside root",
			root:     rootDir,
			path:     filepath.Join(rootDir, "."),
			expected: true,
		},
		{
			name:     "Relative path going outside root",
			root:     rootDir,
			path:     filepath.Join(rootDir, ".."),
			expected: false,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			// Need to import "os"
			if err := os.MkdirAll(filepath.Dir(tc.path), 0755); err != nil {
				t.Fatalf("Failed to create test directories for path %s: %v", tc.path, err)
			}

			inside, err := IsPathInsideRoot(tc.root, tc.path)

			if tc.expectError {
				if err == nil {
					t.Errorf("Expected an error, but got none")
				}
			} else {
				if err != nil {
					t.Errorf("Did not expect an error, but got: %v", err)
				}
				if inside != tc.expected {
					t.Errorf("Expected %v, but got %v", tc.expected, inside)
				}
			}
		})
	}
}
