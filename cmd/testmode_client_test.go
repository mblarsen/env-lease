package cmd

import (
	"io"
	"os"
	"strings"
	"testing"
)

func captureStdout(t *testing.T, fn func()) string {
	t.Helper()

	oldStdout := os.Stdout
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatalf("failed to create stdout pipe: %v", err)
	}
	os.Stdout = w
	defer func() {
		os.Stdout = oldStdout
	}()

	fn()

	if err := w.Close(); err != nil {
		t.Fatalf("failed to close stdout writer: %v", err)
	}
	out, err := io.ReadAll(r)
	if err != nil {
		t.Fatalf("failed to read stdout: %v", err)
	}
	if err := r.Close(); err != nil {
		t.Fatalf("failed to close stdout reader: %v", err)
	}
	return string(out)
}

func TestStatusCommandTestModeNoPanic(t *testing.T) {
	t.Setenv("ENV_LEASE_TEST", "1")

	var runErr error
	output := captureStdout(t, func() {
		defer func() {
			if r := recover(); r != nil {
				t.Fatalf("status command panicked in test mode: %v", r)
			}
		}()

		runErr = statusCmd.RunE(statusCmd, []string{})
	})

	if runErr != nil {
		t.Fatalf("status command returned error in test mode: %v", runErr)
	}
	if !strings.Contains(output, "Status command running in test mode.") {
		t.Fatalf("expected test mode status message, got %q", output)
	}
}

func TestDaemonCleanupCommandTestModeNoPanic(t *testing.T) {
	t.Setenv("ENV_LEASE_TEST", "1")

	var runErr error
	output := captureStdout(t, func() {
		defer func() {
			if r := recover(); r != nil {
				t.Fatalf("daemon cleanup command panicked in test mode: %v", r)
			}
		}()

		runErr = cleanupCmd.RunE(cleanupCmd, []string{})
	})

	if runErr != nil {
		t.Fatalf("daemon cleanup returned error in test mode: %v", runErr)
	}
	if !strings.Contains(output, "Cleanup command running in test mode.") {
		t.Fatalf("expected test mode cleanup message, got %q", output)
	}
}
