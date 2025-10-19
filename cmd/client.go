package cmd

import (
	"fmt"
	"os"

	"github.com/mblarsen/env-lease/internal/ipc"
)

func newClient() *ipc.Client {
	if os.Getenv("ENV_LEASE_TEST") == "1" {
		return nil
	}
	secret, err := getSecret()
	if err != nil {
		handleClientError(fmt.Errorf("failed to get secret: %w", err))
	}
	return ipc.NewClient(getSocketPath(), secret)
}
