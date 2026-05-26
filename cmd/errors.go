package cmd

import (
	"errors"
	"log/slog"
	"os"

	"github.com/mblarsen/env-lease/internal/ipc"
	"github.com/mblarsen/env-lease/internal/presentation"
)

func handleClientError(err error) {
	slog.Error("an ipc error occurred", "err", err)
	var connErr *ipc.ConnectionError
	if errors.As(err, &connErr) {
		presenter.Print(os.Stderr, presentation.MessageDaemonOffline)
	} else {
		presenter.Print(os.Stderr, presentation.MessageDaemonConnectionFailed)
	}
	os.Exit(1)
}
