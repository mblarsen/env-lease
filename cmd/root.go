package cmd

import (
	"errors"
	"fmt"
	"github.com/mblarsen/env-lease/internal/ipc"
	"github.com/spf13/cobra"
	"os"
)

var rootCmd = &cobra.Command{
	Use:   "env-lease",
	Short: "A CLI for managing temporary, leased secrets in environment files.",
	Long: `env-lease is a tool that automates the lifecycle of secrets in local
development files. It fetches secrets, injects them into files, and revokes
them after a specified lease duration.`,
}

func Execute() {
	if err := rootCmd.Execute(); err != nil {
		// Check if it's a connection error, and if so, print a cleaner message.
		var connErr *ipc.ConnectionError
		if errors.As(err, &connErr) {
			fmt.Fprintf(os.Stderr, "Error: %s\n", connErr)
		} else {
			// cobra will print the error, so we don't need to.
		}
		os.Exit(1)
	}
}
