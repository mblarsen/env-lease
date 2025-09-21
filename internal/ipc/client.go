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
func (c *Client) Send(payload interface{}, responsePayload interface{}) error {
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

	// Decode the response
	if responsePayload != nil {
		var resp Response
		if err := json.NewDecoder(conn).Decode(&resp); err != nil {
			return fmt.Errorf("failed to decode response: %w", err)
		}

		if resp.Error != "" {
			return fmt.Errorf("server error: %s", resp.Error)
		}

		if err := json.Unmarshal(resp.Payload, responsePayload); err != nil {
			return fmt.Errorf("failed to unmarshal response payload: %w", err)
		}
	}

	return nil
}
