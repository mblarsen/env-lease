package cmd

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"
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
		fmt.Fprintf(os.Stderr, "Whoops. There was an error while executing your CLI '%s'", err)
		os.Exit(1)
	}
}
