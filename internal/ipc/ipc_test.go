package ipc

import (
	"encoding/json"
	"errors"
	"net"
	"os"
	"path/filepath"
	"testing"
)

func TestClientServerTypedRequestResponse(t *testing.T) {
	server, client := newTestIPC(t, []byte("shared-secret"))
	commandCh := make(chan Command, 1)

	go func() {
		_ = server.Listen(func(payload []byte) ([]byte, error) {
			command, err := DecodeCommand(payload)
			if err != nil {
				return nil, err
			}
			commandCh <- command
			return json.Marshal(GrantResponse{Messages: []string{"granted"}})
		})
	}()

	var resp GrantResponse
	if err := client.Send(GrantRequest{Leases: []Lease{{Source: "test"}}}, &resp); err != nil {
		t.Fatalf("client send failed: %v", err)
	}
	if got, want := resp.Messages, []string{"granted"}; len(got) != len(want) || got[0] != want[0] {
		t.Fatalf("response messages = %v, want %v", got, want)
	}
	if got := <-commandCh; got != CommandGrant {
		t.Fatalf("command = %q, want %q", got, CommandGrant)
	}
}

func TestServerRejectsInvalidSignature(t *testing.T) {
	server, _ := newTestIPC(t, []byte("server-secret"))
	handled := make(chan struct{}, 1)

	go func() {
		_ = server.Listen(func(payload []byte) ([]byte, error) {
			handled <- struct{}{}
			return nil, nil
		})
	}()

	badClient := NewClient(server.SocketPath(), []byte("client-secret"))
	var resp StatusResponse
	err := badClient.Send(StatusRequest{}, &resp)
	var serverErr *ServerError
	if !errors.As(err, &serverErr) {
		t.Fatalf("error = %v, want ServerError", err)
	}
	if serverErr.Message != ErrInvalidSignature.Error() {
		t.Fatalf("server error message = %q, want %q", serverErr.Message, ErrInvalidSignature.Error())
	}
	select {
	case <-handled:
		t.Fatal("handler ran for an invalid signature")
	default:
	}
}

func TestClientReturnsDaemonError(t *testing.T) {
	server, client := newTestIPC(t, []byte("shared-secret"))
	go func() {
		_ = server.Listen(func(payload []byte) ([]byte, error) {
			return nil, errors.New("boom")
		})
	}()

	err := client.Send(StatusRequest{}, &StatusResponse{})
	var serverErr *ServerError
	if !errors.As(err, &serverErr) {
		t.Fatalf("error = %v, want ServerError", err)
	}
	if serverErr.Message != "boom" {
		t.Fatalf("server error message = %q, want boom", serverErr.Message)
	}
}

func TestClientEOFBehavior(t *testing.T) {
	tmpDir, err := os.MkdirTemp("/tmp", "env-lease-ipc-*")
	if err != nil {
		t.Fatalf("temp dir: %v", err)
	}
	t.Cleanup(func() { _ = os.RemoveAll(tmpDir) })
	socketPath := filepath.Join(tmpDir, "eof.sock")
	listener, err := net.Listen("unix", socketPath)
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	defer listener.Close()

	go func() {
		for {
			conn, err := listener.Accept()
			if err != nil {
				return
			}
			_ = conn.Close()
		}
	}()

	client := NewClient(socketPath, []byte("shared-secret"))
	if err := client.Send(StatusRequest{}, nil); err != nil {
		t.Fatalf("nil response payload error = %v, want nil", err)
	}
	if err := client.Send(StatusRequest{}, &StatusResponse{}); !errors.Is(err, ErrNoResponse) {
		t.Fatalf("typed response error = %v, want ErrNoResponse", err)
	}
}

func TestClientRequiresPayloadForTypedResponse(t *testing.T) {
	server, client := newTestIPC(t, []byte("shared-secret"))
	go func() {
		_ = server.Listen(func(payload []byte) ([]byte, error) {
			return nil, nil
		})
	}()

	if err := client.Send(StatusRequest{}, &StatusResponse{}); !errors.Is(err, ErrEmptyPayload) {
		t.Fatalf("error = %v, want ErrEmptyPayload", err)
	}
}

func TestConnectionErrorUserMessage(t *testing.T) {
	client := NewClient(filepath.Join(t.TempDir(), "missing.sock"), []byte("shared-secret"))
	err := client.Send(StatusRequest{}, &StatusResponse{})
	var connErr *ConnectionError
	if !errors.As(err, &connErr) {
		t.Fatalf("error = %v, want ConnectionError", err)
	}

	want := "Error: env-lease daemon is not running. Please start it with 'env-lease daemon start'."
	if got := UserMessage(err); got != want {
		t.Fatalf("UserMessage = %q, want %q", got, want)
	}
}

func newTestIPC(t *testing.T, secret []byte) (*Server, *Client) {
	t.Helper()
	tmpDir, err := os.MkdirTemp("/tmp", "env-lease-ipc-*")
	if err != nil {
		t.Fatalf("temp dir: %v", err)
	}
	t.Cleanup(func() { _ = os.RemoveAll(tmpDir) })
	socketPath := filepath.Join(tmpDir, "test.sock")
	server, err := NewServer(socketPath, secret)
	if err != nil {
		t.Fatalf("failed to create server: %v", err)
	}
	t.Cleanup(func() { _ = server.Close() })
	return server, NewClient(socketPath, secret)
}
