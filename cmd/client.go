package cmd

import (
	"fmt"
	"github.com/mblarsen/env-lease/internal/ipc"
)

func newClient() *ipc.Client {
	secret, err := getSecret()
	if err != nil {
		handleClientError(fmt.Errorf("failed to get secret: %w", err))
	}
	return ipc.NewClient(getSocketPath(), secret)
}
