package cmd

import (
	"fmt"
	"log/slog"
	"os"

	"github.com/mblarsen/env-lease/internal/ipc"
)

func handleClientError(err error) {
	slog.Error("an ipc error occurred", "err", err)
	_, _ = fmt.Fprintln(os.Stderr, ipc.UserMessage(err))
	os.Exit(1)
}
