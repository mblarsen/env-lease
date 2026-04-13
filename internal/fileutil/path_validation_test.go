package fileutil

import (
	"os"
	"path/filepath"
	"testing"
)

func TestIsPathInsideRoot(t *testing.T) {
	tmpDir := t.TempDir()
	rootDir := filepath.Join(tmpDir, "root")
	insideDir := filepath.Join(rootDir, "inside")
	outsideDir := filepath.Join(tmpDir, "outside")

	if err := os.MkdirAll(insideDir, 0o755); err != nil {
		t.Fatalf("failed to create inside dir: %v", err)
	}
	if err := os.MkdirAll(outsideDir, 0o755); err != nil {
		t.Fatalf("failed to create outside dir: %v", err)
	}

	testCases := []struct {
		name     string
		path     string
		expected bool
	}{
		{
			name:     "Path inside root",
			path:     filepath.Join(rootDir, "inside", "some", "path"),
			expected: true,
		},
		{
			name:     "Path is the root",
			path:     rootDir,
			expected: true,
		},
		{
			name:     "Path outside root",
			path:     filepath.Join(outsideDir, "some", "path"),
			expected: false,
		},
		{
			name:     "Relative path inside root",
			path:     filepath.Join("inside", "nested", "file.txt"),
			expected: true,
		},
		{
			name:     "Relative path escaping root",
			path:     filepath.Join("..", "outside", "file.txt"),
			expected: false,
		},
		{
			name:     "Non-existent path inside root with existing parent",
			path:     filepath.Join(rootDir, "inside", "new-file.txt"),
			expected: true,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			inside, err := IsPathInsideRoot(rootDir, tc.path)
			if err != nil {
				t.Fatalf("did not expect error: %v", err)
			}
			if inside != tc.expected {
				t.Fatalf("expected %v, got %v", tc.expected, inside)
			}
		})
	}
}

func TestIsPathInsideRoot_SymlinkEscape(t *testing.T) {
	tmpDir := t.TempDir()
	rootDir := filepath.Join(tmpDir, "root")
	outsideDir := filepath.Join(tmpDir, "outside")
	insideTargetDir := filepath.Join(rootDir, "real-inside")

	if err := os.MkdirAll(rootDir, 0o755); err != nil {
		t.Fatalf("failed to create root dir: %v", err)
	}
	if err := os.MkdirAll(outsideDir, 0o755); err != nil {
		t.Fatalf("failed to create outside dir: %v", err)
	}
	if err := os.MkdirAll(insideTargetDir, 0o755); err != nil {
		t.Fatalf("failed to create inside target dir: %v", err)
	}

	outsideLink := filepath.Join(rootDir, "outside-link")
	if err := os.Symlink(outsideDir, outsideLink); err != nil {
		t.Skipf("symlink not supported on this platform: %v", err)
	}

	insideLink := filepath.Join(rootDir, "inside-link")
	if err := os.Symlink(insideTargetDir, insideLink); err != nil {
		t.Skipf("symlink not supported on this platform: %v", err)
	}

	testCases := []struct {
		name     string
		path     string
		expected bool
	}{
		{
			name:     "Symlink to outside root is rejected",
			path:     filepath.Join(outsideLink, "secret.txt"),
			expected: false,
		},
		{
			name:     "Symlink traversal with dotdot escaping root is rejected",
			path:     outsideLink + string(os.PathSeparator) + ".." + string(os.PathSeparator) + "escaped.txt",
			expected: false,
		},
		{
			name:     "Symlink to inside root is allowed",
			path:     filepath.Join(insideLink, "secret.txt"),
			expected: true,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			inside, err := IsPathInsideRoot(rootDir, tc.path)
			if err != nil {
				t.Fatalf("did not expect error: %v", err)
			}
			if inside != tc.expected {
				t.Fatalf("expected %v, got %v", tc.expected, inside)
			}
		})
	}
}
