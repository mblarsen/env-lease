package fileutil

import (
	"os"
	"path/filepath"
	"testing"
)

func TestAtomicWriteFile(t *testing.T) {
	dir := t.TempDir()
	filename := filepath.Join(dir, "test.txt")
	data := []byte("hello world")
	perm := os.FileMode(0644)

	err := AtomicWriteFile(filename, data, perm)
	if err != nil {
		t.Fatalf("AtomicWriteFile failed: %v", err)
	}

	// Verify content
	content, err := os.ReadFile(filename)
	if err != nil {
		t.Fatalf("failed to read file: %v", err)
	}
	if string(content) != string(data) {
		t.Errorf("expected content %q, got %q", data, content)
	}

	// Verify permissions
	info, err := os.Stat(filename)
	if err != nil {
		t.Fatalf("failed to stat file: %v", err)
	}
	if info.Mode() != perm {
		t.Errorf("expected file mode %v, got %v", perm, info.Mode())
	}
}
