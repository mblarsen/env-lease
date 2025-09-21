package ipc

import (
	"crypto/rand"
	"os"
)

// GetOrCreateSecret reads a secret from a file, or creates it if it doesn't exist.
func GetOrCreateSecret(path string) ([]byte, error) {
	if _, err := os.Stat(path); os.IsNotExist(err) {
		secret := make([]byte, 32)
		if _, err := rand.Read(secret); err != nil {
			return nil, err
		}
		if err := os.WriteFile(path, secret, 0600); err != nil {
			return nil, err
		}
		return secret, nil
	}

	return os.ReadFile(path)
}
