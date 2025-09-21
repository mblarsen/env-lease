package ipc

import (
	"crypto/rand"
	"path/filepath"
	"testing"
	"time"
)

func TestIPC(t *testing.T) {
	socketPath := filepath.Join(t.TempDir(), "test.sock")
	secret := make([]byte, 32)
	if _, err := rand.Read(secret); err != nil {
		t.Fatalf("failed to generate secret: %v", err)
	}

	server, err := NewServer(socketPath, secret)
	if err != nil {
		t.Fatalf("failed to create server: %v", err)
	}
	defer server.Close()

	handlerChan := make(chan []byte, 1)
	handler := func(payload []byte) ([]byte, error) {
		handlerChan <- payload
		return nil, nil
	}
	go server.Listen(handler)

	// Allow server to start
	time.Sleep(100 * time.Millisecond)

	t.Run("successful communication", func(t *testing.T) {
		client := NewClient(socketPath, secret)
		payload := GrantRequest{
			Leases: []Lease{{Source: "test"}},
		}
		if err := client.Send(payload, nil); err != nil {
			t.Fatalf("client send failed: %v", err)
		}

		select {
		case receivedPayload := <-handlerChan:
			// Just check that we got something
			if len(receivedPayload) == 0 {
				t.Error("handler received empty payload")
			}
		case <-time.After(1 * time.Second):
			t.Fatal("handler did not receive payload in time")
		}
	})

	t.Run("invalid signature", func(t *testing.T) {
		badSecret := make([]byte, 32)
		client := NewClient(socketPath, badSecret)
		payload := GrantRequest{
			Leases: []Lease{{Source: "test"}},
		}
		if err := client.Send(payload, nil); err != nil {
			t.Fatalf("client send failed: %v", err)
		}

		select {
		case <-handlerChan:
			t.Fatal("handler should not have received payload")
		case <-time.After(100 * time.Millisecond):
			// Expected timeout
		}
	})
}
