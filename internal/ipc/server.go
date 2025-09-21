package ipc

import (
	"encoding/json"
	"fmt"
	"net"
	"os"
)

// Server is the IPC server.
type Server struct {
	listener net.Listener
	secret   []byte
}

// NewServer creates a new IPC server.
func NewServer(socketPath string, secret []byte) (*Server, error) {
	if err := os.RemoveAll(socketPath); err != nil {
		return nil, err
	}

	listener, err := net.Listen("unix", socketPath)
	if err != nil {
		return nil, err
	}

	return &Server{
		listener: listener,
		secret:   secret,
	}, nil
}

// Listen starts the server's listening loop.
func (s *Server) Listen(handler func(payload []byte) error) error {
	for {
		conn, err := s.listener.Accept()
		if err != nil {
			return err
		}
		go s.handleConnection(conn, handler)
	}
}

func (s *Server) handleConnection(conn net.Conn, handler func(payload []byte) error) {
	defer conn.Close()

	var req Request
	if err := json.NewDecoder(conn).Decode(&req); err != nil {
		fmt.Fprintf(os.Stderr, "failed to decode request: %v\n", err)
		return
	}

	if err := Verify(req.Payload, req.Signature, s.secret); err != nil {
		fmt.Fprintf(os.Stderr, "invalid signature: %v\n", err)
		return
	}

	if err := handler(req.Payload); err != nil {
		fmt.Fprintf(os.Stderr, "handler error: %v\n", err)
		// Optionally, send error back to client
	}
}

// Close closes the server's listener.
func (s *Server) Close() error {
	return s.listener.Close()
}
