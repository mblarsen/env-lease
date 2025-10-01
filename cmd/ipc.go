package cmd

import (
	"fmt"

	"github.com/mblarsen/env-lease/internal/ipc"
	"github.com/mblarsen/env-lease/internal/xdgpath"
)

func getSocketPath() string {
	socketPath, err := xdgpath.RuntimePath("daemon.sock")
	if err != nil {
		panic(fmt.Sprintf("could not determine runtime directory: %v", err))
	}
	return socketPath
}

func getSecret() ([]byte, error) {
	secretPath, err := xdgpath.StatePath("auth.token")
	if err != nil {
		panic(fmt.Sprintf("could not determine state directory: %v", err))
	}
	return ipc.GetOrCreateSecret(secretPath)
}
