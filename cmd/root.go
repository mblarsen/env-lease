package cmd

import (
	"context"
	"log/slog"
	"os"
	"strings"
	"time"

	"github.com/charmbracelet/fang"
	"github.com/lmittmann/tint"
	"github.com/spf13/cobra"
)

var rootCmd = &cobra.Command{
	Use:   "env-lease",
	Short: "A CLI for managing temporary, leased secrets in environment files.",
	Long: `env-lease is a tool that automates the lifecycle of secrets in local
development files. It fetches secrets, injects them into files, and revokes
them after a specified lease duration.`,
	SilenceUsage: true,
	PersistentPreRun: func(cmd *cobra.Command, args []string) {
		logLevel := slog.LevelWarn
		switch strings.ToLower(os.Getenv("ENV_LEASE_LOG_LEVEL")) {
		case "debug":
			logLevel = slog.LevelDebug
		case "info":
			logLevel = slog.LevelInfo
		case "error":
			logLevel = slog.LevelError
		}

		slog.SetDefault(slog.New(
			tint.NewHandler(os.Stderr, &tint.Options{
				Level:      logLevel,
				TimeFormat: time.Kitchen,
			}),
		))
	},
}

func Execute() {
	if err := fang.Execute(context.Background(), rootCmd); err != nil {
		os.Exit(1)
	}
}
