package cmd

import (
	"github.com/mblarsen/env-lease/internal/ipc"
	"path/filepath"
	"os"
)

func getSocketPath() string {
	configDir := filepath.Join(os.Getenv("HOME"), ".config", "env-lease")
	return filepath.Join(configDir, "daemon.sock")
}

func getSecret() ([]byte, error) {
	configDir := filepath.Join(os.Getenv("HOME"), ".config", "env-lease")
	secretPath := filepath.Join(configDir, "auth.token")
	return ipc.GetOrCreateSecret(secretPath)
}
