package ipc

import (
	"encoding/json"
	"net"
)

// Client is the IPC client.
type Client struct {
	socketPath string
	secret     []byte
}

// NewClient creates a new IPC client.
func NewClient(socketPath string, secret []byte) *Client {
	return &Client{
		socketPath: socketPath,
		secret:     secret,
	}
}

// Send sends a request to the server.
func (c *Client) Send(payload interface{}) error {
	req, err := NewRequest(payload, c.secret)
	if err != nil {
		return err
	}

	conn, err := net.Dial("unix", c.socketPath)
	if err != nil {
		return err
	}
	defer conn.Close()

	if err := json.NewEncoder(conn).Encode(req); err != nil {
		return err
	}

	return nil
}
