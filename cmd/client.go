package cmd

import (
	"fmt"
	"os"

	"github.com/mblarsen/env-lease/internal/ipc"
)

func newIPCClient() *ipc.Client {
	if os.Getenv("ENV_LEASE_TEST") == "1" {
		return nil
	}
	secret, err := getSecret()
	if err != nil {
		handleClientError(fmt.Errorf("failed to get secret: %w", err))
	}
	return ipc.NewClient(getSocketPath(), secret)
}

func ensureDaemonClient() *ipc.Client {
	client := newIPCClient()
	if client == nil {
		return nil
	}

	var resp ipc.StatusResponse
	if err := client.Send(ipc.StatusRequest{Command: "status"}, &resp); err != nil {
		handleClientError(err)
	}
	return client
}
