package cmd

import (
	"github.com/mblarsen/env-lease/internal/config"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestWriteLease(t *testing.T) {
	tempDir := t.TempDir()

	t.Run("create new env file", func(t *testing.T) {
		destFile := filepath.Join(tempDir, ".env.create")
		lease := config.Lease{
			Destination: destFile,
			LeaseType:   "env",
			Variable:    "MY_VAR",
		}
		created, err := writeLease(lease, "my_value", false)
		if err != nil {
			t.Fatalf("writeLease failed: %v", err)
		}
		if !created {
			t.Fatal("expected created to be true")
		}
		content, _ := os.ReadFile(destFile)
		if string(content) != "export MY_VAR=my_value\n" {
			t.Fatalf("unexpected content: %s", string(content))
		}
	})

	t.Run("append to existing env file", func(t *testing.T) {
		destFile := filepath.Join(tempDir, ".env.append")
		_ = os.WriteFile(destFile, []byte("export EXISTING_VAR=123\n"), 0644)
		lease := config.Lease{
			Destination: destFile,
			LeaseType:   "env",
			Variable:    "MY_VAR",
		}
		created, err := writeLease(lease, "my_value", false)
		if err != nil {
			t.Fatalf("writeLease failed: %v", err)
		}
		if created {
			t.Fatal("expected created to be false")
		}
		content, _ := os.ReadFile(destFile)
		if !strings.Contains(string(content), "export EXISTING_VAR=123") {
			t.Fatal("existing content not preserved")
		}
		if !strings.Contains(string(content), "export MY_VAR=my_value") {
			t.Fatal("new content not appended")
		}
	})

	t.Run("override existing var in env file", func(t *testing.T) {
		destFile := filepath.Join(tempDir, ".env.override")
		_ = os.WriteFile(destFile, []byte("export MY_VAR=old_value\n"), 0644)
		lease := config.Lease{
			Destination: destFile,
			LeaseType:   "env",
			Variable:    "MY_VAR",
		}
		_, err := writeLease(lease, "new_value", true)
		if err != nil {
			t.Fatalf("writeLease failed: %v", err)
		}
		content, _ := os.ReadFile(destFile)
		if string(content) != "export MY_VAR=new_value\n" {
			t.Fatalf("unexpected content: %q", string(content))
		}
	})

	t.Run("do not override existing var in env file", func(t *testing.T) {
		destFile := filepath.Join(tempDir, ".env.no_override")
		_ = os.WriteFile(destFile, []byte("export MY_VAR=old_value\n"), 0644)
		lease := config.Lease{
			Destination: destFile,
			LeaseType:   "env",
			Variable:    "MY_VAR",
		}
		_, err := writeLease(lease, "new_value", false)
		if err != nil {
			t.Fatalf("writeLease failed: %v", err)
		}
		content, _ := os.ReadFile(destFile)
		if string(content) != "export MY_VAR=old_value\n" {
			t.Fatalf("unexpected content: %q", string(content))
		}
	})
}
