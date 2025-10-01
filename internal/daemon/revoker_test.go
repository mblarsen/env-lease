package daemon

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/mblarsen/env-lease/internal/config"
)

func TestFileRevoker_Revoke(t *testing.T) {
	tempDir := t.TempDir()

	t.Run("file lease, file exists", func(t *testing.T) {
		// Create a dummy file to be revoked
		filePath := filepath.Join(tempDir, "secret.txt")
		err := os.WriteFile(filePath, []byte("secret"), 0644)
		if err != nil {
			t.Fatalf("failed to create test file: %v", err)
		}

		lease := &config.Lease{
			LeaseType:   "file",
			Destination: filePath,
		}

		revoker := &FileRevoker{}
		err = revoker.Revoke(lease)
		if err != nil {
			t.Fatalf("expected no error, but got: %v", err)
		}

		// Check that the file was deleted
		if _, err := os.Stat(filePath); !os.IsNotExist(err) {
			t.Fatal("expected file to be deleted, but it still exists")
		}
	})

	t.Run("file lease, file already deleted", func(t *testing.T) {
		filePath := filepath.Join(tempDir, "already-deleted.txt")

		lease := &config.Lease{
			LeaseType:   "file",
			Destination: filePath,
		}

		revoker := &FileRevoker{}
		err := revoker.Revoke(lease)
		if err != nil {
			t.Fatalf("expected no error when file is already deleted, but got: %v", err)
		}
	})
}
