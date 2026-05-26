package ipc

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"time"
)

// Command identifies the daemon operation carried by an IPC request payload.
type Command string

const (
	// CommandGrant registers granted Leases with the Daemon.
	CommandGrant Command = "grant"
	// CommandStatus asks the Daemon for active Lease state.
	CommandStatus Command = "status"
	// CommandCleanup asks the Daemon to reconcile orphaned Leases.
	CommandCleanup Command = "cleanup"
	// CommandRevoke asks the Daemon to revoke active Leases.
	CommandRevoke Command = "revoke"
)

// Request represents a signed request sent from the CLI to the daemon.
type Request struct {
	Signature string
	Payload   []byte
}

// CommandCarrier is implemented by typed daemon request payloads.
type CommandCarrier interface {
	IPCCommand() Command
}

// GrantRequest is the payload for a grant request.
type GrantRequest struct {
	Command    string
	Leases     []Lease
	Override   bool
	Append     bool
	ConfigFile string
}

// IPCCommand returns the daemon command represented by GrantRequest.
func (GrantRequest) IPCCommand() Command { return CommandGrant }

// GrantResponse is the payload for a grant response.
type GrantResponse struct {
	Messages []string
}

// StatusRequest is the payload for a status request.
type StatusRequest struct {
	Command    string
	ConfigFile string
}

// IPCCommand returns the daemon command represented by StatusRequest.
func (StatusRequest) IPCCommand() Command { return CommandStatus }

// StatusResponse is the payload for a status response.
type StatusResponse struct {
	Leases []Lease
}

// CleanupRequest is the payload for a cleanup request.
type CleanupRequest struct {
	Command string
}

// IPCCommand returns the daemon command represented by CleanupRequest.
func (CleanupRequest) IPCCommand() Command { return CommandCleanup }

// CleanupResponse is the payload for a cleanup response.
type CleanupResponse struct {
	Messages []string
}

// RevokeRequest is the payload for a revoke request.
type RevokeRequest struct {
	Command    string
	ConfigFile string
	All        bool
	Leases     []Lease
}

// IPCCommand returns the daemon command represented by RevokeRequest.
func (RevokeRequest) IPCCommand() Command { return CommandRevoke }

// RevokeResponse is the payload for a revoke response.
type RevokeResponse struct {
	Messages      []string
	ShellCommands []string
}

// Lease is a simplified lease structure for IPC.
type Lease struct {
	Source       string
	Destination  string
	Duration     string
	LeaseType    string
	Variable     string
	Format       string
	Transform    []string
	FileMode     string
	ExpiresAt    time.Time
	ConfigFile   string
	OpAccount    string
	ParentSource string
}

// Sign creates a signature for the payload.
func Sign(payload []byte, secret []byte) string {
	mac := hmac.New(sha256.New, secret)
	mac.Write(payload)
	return hex.EncodeToString(mac.Sum(nil))
}

// Verify checks the signature of the payload.
func Verify(payload []byte, signature string, secret []byte) error {
	expectedSignature := Sign(payload, secret)
	if !hmac.Equal([]byte(signature), []byte(expectedSignature)) {
		return ErrInvalidSignature
	}
	return nil
}

// Response represents a response sent from the daemon to the CLI.
type Response struct {
	Error   string          `json:"error,omitempty"`
	Payload json.RawMessage `json:"payload,omitempty"`
}

var (
	// ErrInvalidSignature means the request signature did not match the payload.
	ErrInvalidSignature = errors.New("invalid signature")
	// ErrNoResponse means the daemon closed the connection before sending a response.
	ErrNoResponse = errors.New("daemon closed connection without a response")
	// ErrEmptyPayload means the daemon returned success without a payload for a typed response.
	ErrEmptyPayload = errors.New("daemon returned no response payload")
)

// ConnectionError is a custom error for IPC connection errors.
type ConnectionError struct {
	SocketPath string
	Err        error
}

func (e *ConnectionError) Error() string {
	return fmt.Sprintf("could not connect to the env-lease daemon at %s. Is the daemon running?", e.SocketPath)
}

func (e *ConnectionError) Unwrap() error {
	return e.Err
}

// ServerError wraps an error response returned by the daemon.
type ServerError struct {
	Message string
}

func (e *ServerError) Error() string {
	return fmt.Sprintf("server error: %s", e.Message)
}

// UserMessage maps low-level IPC failures to stable CLI-facing text.
func UserMessage(err error) string {
	var connErr *ConnectionError
	if errors.As(err, &connErr) {
		return "Error: env-lease daemon is not running. Please start it with 'env-lease daemon start'."
	}
	return "Error: could not connect to the env-lease daemon. Is it running?"
}

// NewRequest creates a new signed request.
func NewRequest(payload any, secret []byte) (*Request, error) {
	payloadBytes, err := json.Marshal(normalizeCommand(payload))
	if err != nil {
		return nil, err
	}

	signature := Sign(payloadBytes, secret)
	return &Request{
		Signature: signature,
		Payload:   payloadBytes,
	}, nil
}

func normalizeCommand(payload any) any {
	switch req := payload.(type) {
	case GrantRequest:
		if req.Command == "" {
			req.Command = string(req.IPCCommand())
		}
		return req
	case *GrantRequest:
		if req != nil && req.Command == "" {
			copy := *req
			copy.Command = string(req.IPCCommand())
			return copy
		}
	case StatusRequest:
		if req.Command == "" {
			req.Command = string(req.IPCCommand())
		}
		return req
	case *StatusRequest:
		if req != nil && req.Command == "" {
			copy := *req
			copy.Command = string(req.IPCCommand())
			return copy
		}
	case CleanupRequest:
		if req.Command == "" {
			req.Command = string(req.IPCCommand())
		}
		return req
	case *CleanupRequest:
		if req != nil && req.Command == "" {
			copy := *req
			copy.Command = string(req.IPCCommand())
			return copy
		}
	case RevokeRequest:
		if req.Command == "" {
			req.Command = string(req.IPCCommand())
		}
		return req
	case *RevokeRequest:
		if req != nil && req.Command == "" {
			copy := *req
			copy.Command = string(req.IPCCommand())
			return copy
		}
	}
	return payload
}

// DecodeCommand extracts the command from a signed request payload.
func DecodeCommand(payload []byte) (Command, error) {
	var req struct {
		Command string
	}
	if err := json.Unmarshal(payload, &req); err != nil {
		return "", fmt.Errorf("failed to unmarshal command: %w", err)
	}
	return Command(req.Command), nil
}
