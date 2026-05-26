package ipc

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
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

// CheckDaemon verifies that the daemon is reachable and can answer status.
func (c *Client) CheckDaemon() error {
	var resp StatusResponse
	return c.Send(StatusRequest{}, &resp)
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

	var resp Response
	if err := json.NewDecoder(conn).Decode(&resp); err != nil {
		if errors.Is(err, io.EOF) {
			if responsePayload == nil {
				return nil
			}
			return ErrNoResponse
		}
		return fmt.Errorf("failed to decode response: %w", err)
	}

	if resp.Error != "" {
		return &ServerError{Message: resp.Error}
	}

	if responsePayload == nil {
		return nil
	}
	if len(resp.Payload) == 0 {
		return ErrEmptyPayload
	}
	if err := json.Unmarshal(resp.Payload, responsePayload); err != nil {
		return fmt.Errorf("failed to unmarshal response payload: %w", err)
	}

	return nil
}
