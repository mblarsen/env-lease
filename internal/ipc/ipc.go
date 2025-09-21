package ipc

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
)

// Request represents a request sent from the CLI to the daemon.
type Request struct {
	Signature string
	Payload   []byte
}

// GrantRequest is the payload for a grant request.
type GrantRequest struct {
	Command string
	Leases  []Lease
}

// Lease is a simplified lease structure for IPC.
type Lease struct {
	Source      string
	Destination string
	Duration    string
	LeaseType   string
	Variable    string
	Format      string
	Encoding    string
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
		return fmt.Errorf("invalid signature")
	}
	return nil
}

// Response represents a response sent from the daemon to the CLI.
type Response struct {
	Error   string          `json:"error,omitempty"`
	Payload json.RawMessage `json:"payload,omitempty"`
}

// NewRequest creates a new signed request.
func NewRequest(payload any, secret []byte) (*Request, error) {
	payloadBytes, err := json.Marshal(payload)
	if err != nil {
		return nil, err
	}

	signature := Sign(payloadBytes, secret)
	return &Request{
		Signature: signature,
		Payload:   payloadBytes,
	}, nil
}
