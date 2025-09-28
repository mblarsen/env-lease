package cmd

import (
	"fmt"
	"log/slog"
	"os"
)

func handleClientError(err error) {
	slog.Error("an ipc error occurred", "err", err)
	_, _ = fmt.Fprintln(os.Stderr, "Error: could not connect to the env-lease daemon. Is it running?")
	os.Exit(1)
}
