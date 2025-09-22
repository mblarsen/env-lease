package daemon

import (
	"context"
	"crypto/rand"
	"github.com/mblarsen/env-lease/internal/ipc"
	"path/filepath"
	"testing"
	"time"
)

func TestDaemonServer(t *testing.T) {
	socketPath := filepath.Join(t.TempDir(), "test.sock")
	secret := make([]byte, 32)
	if _, err := rand.Read(secret); err != nil {
		t.Fatalf("failed to generate secret: %v", err)
	}

	state := NewState()
	clock := &RealClock{}
	ipcServer, err := ipc.NewServer(socketPath, secret)
	if err != nil {
		t.Fatalf("failed to create ipc server: %v", err)
	}
	revoker := &mockRevoker{}
	daemon := NewDaemon(state, "/dev/null", clock, ipcServer, revoker)
	go daemon.Run(context.Background())
	defer ipcServer.Close()

	time.Sleep(100 * time.Millisecond) // Allow server to start

	client := ipc.NewClient(socketPath, secret)

	t.Run("grant lease", func(t *testing.T) {
		req := ipc.GrantRequest{
			Command: "grant",
			Leases: []ipc.Lease{
				{Source: "test", Destination: "test.txt", Duration: "1h"},
			},
		}
		if err := client.Send(req, nil); err != nil {
			t.Fatalf("failed to send grant request: %v", err)
		}

		time.Sleep(50 * time.Millisecond) // Allow handler to run

		if len(daemon.state.Leases) != 1 {
			t.Errorf("expected 1 lease, got %d", len(daemon.state.Leases))
		}
	})

	t.Run("revoke lease", func(t *testing.T) {
		req := struct{ Command string }{Command: "revoke"}
		if err := client.Send(req, nil); err != nil {
			t.Fatalf("failed to send revoke request: %v", err)
		}

		time.Sleep(50 * time.Millisecond) // Allow handler to run

		if len(daemon.state.Leases) != 0 {
			t.Errorf("expected 0 leases, got %d", len(daemon.state.Leases))
		}
	})
}
