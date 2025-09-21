package ipc

import (
	"encoding/json"
	"fmt"
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

// Send sends a request to the server and decodes the response.
func (c *Client) Send(payload any, responsePayload any) error {
	req, err := NewRequest(payload, c.secret)
	if err != nil {
		return err
	}

	conn, err := net.Dial("unix", c.socketPath)
	if err != nil {
		return &ConnectionError{SocketPath: c.socketPath, Err: err}
	}
	defer conn.Close()

	if err := json.NewEncoder(conn).Encode(req); err != nil {
		return err
	}

	// Always read the response to properly close the connection and check for errors
	var resp Response
	if err := json.NewDecoder(conn).Decode(&resp); err != nil {
		// If the server sends no body, it's a successful fire-and-forget.
		// We can treat EOF as a success signal in this specific case.
		if err.Error() == "EOF" {
			return nil
		}
		return fmt.Errorf("failed to decode response: %w", err)
	}

	if resp.Error != "" {
		return fmt.Errorf("server error: %s", resp.Error)
	}

	// Only unmarshal a payload if the caller is expecting one
	if responsePayload != nil {
		if err := json.Unmarshal(resp.Payload, responsePayload); err != nil {
			return fmt.Errorf("failed to unmarshal response payload: %w", err)
		}
	}

	return nil
}
