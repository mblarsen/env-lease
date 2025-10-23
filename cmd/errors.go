package cmd

import (
	"errors"
	"fmt"
	"log/slog"
	"os"

	"github.com/mblarsen/env-lease/internal/ipc"
)

func handleClientError(err error) {
	slog.Error("an ipc error occurred", "err", err)
	var connErr *ipc.ConnectionError
	if errors.As(err, &connErr) {
		_, _ = fmt.Fprintln(os.Stderr, "Error: env-lease daemon is not running. Please start it with 'env-lease daemon start'.")
	} else {
		_, _ = fmt.Fprintln(os.Stderr, "Error: could not connect to the env-lease daemon. Is it running?")
	}
	os.Exit(1)
}
