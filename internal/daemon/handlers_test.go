package daemon

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/mblarsen/env-lease/internal/ipc"
)

func TestHandleGrant_Idempotency(t *testing.T) {
	tempDir := t.TempDir()
	state := NewState()
	clock := &mockClock{now: time.Now()}
	revoker := &mockRevoker{}
	daemon := NewDaemon(state, "/dev/null", clock, nil, revoker)

	// Test case for env lease
	t.Run("env lease idempotency", func(t *testing.T) {
		destFile := filepath.Join(tempDir, ".env")
		lease := ipc.Lease{
			Source:      "1password",
			Destination: destFile,
			LeaseType:   "env",
			Variable:    "MY_VAR",
			Value:       "my_value",
			Duration:    "1h",
			Format:      "export %s=%s",
		}
		req := ipc.GrantRequest{
			Command:  "grant",
			Leases:   []ipc.Lease{lease},
			Override: false,
		}
		payload, _ := json.Marshal(req)

		// First grant should succeed
		_, err := daemon.handleGrant(payload)
		if err != nil {
			t.Fatalf("first grant failed: %v", err)
		}

		// Second grant should also succeed (idempotency)
		_, err = daemon.handleGrant(payload)
		if err != nil {
			t.Fatalf("second grant failed: %v", err)
		}

		// Grant with different value should fail
		lease.Value = "new_value"
		req.Leases = []ipc.Lease{lease}
		payload, _ = json.Marshal(req)
		_, err = daemon.handleGrant(payload)
		if err == nil {
			t.Fatal("grant with different value should have failed")
		}
	})

	// Test case for file lease
	t.Run("file lease idempotency", func(t *testing.T) {
		destFile := filepath.Join(tempDir, "my_file.txt")
		lease := ipc.Lease{
			Source:      "1password",
			Destination: destFile,
			LeaseType:   "file",
			Value:       "my_value",
			Duration:    "1h",
		}
		req := ipc.GrantRequest{
			Command:  "grant",
			Leases:   []ipc.Lease{lease},
			Override: false,
		}
		payload, _ := json.Marshal(req)

		// First grant should succeed
		_, err := daemon.handleGrant(payload)
		if err != nil {
			t.Fatalf("first grant failed: %v", err)
		}

		// Second grant should also succeed (idempotency)
		_, err = daemon.handleGrant(payload)
		if err != nil {
			t.Fatalf("second grant failed: %v", err)
		}

		// Grant with different value should fail
		lease.Value = "new_value"
		req.Leases = []ipc.Lease{lease}
		payload, _ = json.Marshal(req)
		_, err = daemon.handleGrant(payload)
		if err == nil {
			t.Fatal("grant with different value should have failed")
		}
	})

	// Test case for override
	t.Run("override with different value", func(t *testing.T) {
		destFile := filepath.Join(tempDir, "override_test.txt")
		lease := ipc.Lease{
			Source:      "1password",
			Destination: destFile,
			LeaseType:   "file",
			Value:       "my_value",
			Duration:    "1h",
		}
		req := ipc.GrantRequest{
			Command:  "grant",
			Leases:   []ipc.Lease{lease},
			Override: false,
		}
		payload, _ := json.Marshal(req)

		// First grant
		_, err := daemon.handleGrant(payload)
		if err != nil {
			t.Fatalf("first grant failed: %v", err)
		}

		// Second grant with different value and override
		lease.Value = "new_value"
		req.Leases = []ipc.Lease{lease}
		req.Override = true
		payload, _ = json.Marshal(req)
		_, err = daemon.handleGrant(payload)
		if err != nil {
			t.Fatalf("grant with override failed: %v", err)
		}

		content, _ := os.ReadFile(destFile)
		if string(content) != "new_value" {
			t.Fatalf("expected content 'new_value', got '%s'", string(content))
		}
	})
}
