package cmd

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"time"

	"github.com/lmittmann/tint"
	"github.com/mblarsen/env-lease/internal/daemon"
	"github.com/mblarsen/env-lease/internal/ipc"
	"github.com/mblarsen/env-lease/internal/xdgpath"
	"github.com/spf13/cobra"
)

var daemonCmd = &cobra.Command{
	Use:   "daemon",
	Short: "Manage the env-lease daemon.",
	Long:  `Manage the env-lease daemon.`,
}

var daemonInstallCmd = &cobra.Command{
	Use:   "install",
	Short: "Install the env-lease daemon.",
	Long:  `Install the env-lease daemon.`,
}

var daemonUninstallCmd = &cobra.Command{
	Use:   "uninstall",
	Short: "Uninstall the env-lease daemon.",
	Long:  `Uninstall the env-lease daemon.`,
}

var daemonStatusCmd = &cobra.Command{
	Use:   "status",
	Short: "Check the status of the daemon.",
	Long:  `Checks whether the daemon is installed and running.`,
}

var daemonReloadCmd = &cobra.Command{
	Use:   "reload",
	Short: "Reload the env-lease daemon.",
	Long:  `Reload the env-lease daemon.`,
}

var runCmd = &cobra.Command{
	Use:   "run",
	Short: "Run the env-lease daemon.",
	Long:  `Run the env-lease daemon.`,
	RunE: func(cmd *cobra.Command, args []string) error {
		// set up logger
		logLevel := slog.LevelInfo
		if levelStr := os.Getenv("ENV_LEASE_LOG_LEVEL"); levelStr != "" {
			var l slog.Level
			if err := l.UnmarshalText([]byte(levelStr)); err == nil {
				logLevel = l
			}
		}
		slog.SetDefault(slog.New(
			tint.NewHandler(os.Stderr, &tint.Options{
				Level:      logLevel,
				TimeFormat: time.Kitchen,
			}),
		))
		slog.Info("Starting daemon...")

		// Configuration paths
		socketPath, err := xdgpath.RuntimePath("daemon.sock")
		if err != nil {
			return fmt.Errorf("failed to get runtime path: %w", err)
		}
		statePath, err := xdgpath.StatePath("state.json")
		if err != nil {
			return fmt.Errorf("failed to get state path: %w", err)
		}
		secretPath, err := xdgpath.StatePath("auth.token")
		if err != nil {
			return fmt.Errorf("failed to get secret path: %w", err)
		}

		// Get or create secret
		secret, err := ipc.GetOrCreateSecret(secretPath)
		if err != nil {
			return err
		}

		// Load state
		state, err := daemon.LoadState(statePath)
		if err != nil {
			slog.Warn("No state file found, initializing new state.")
		} else {
			slog.Info("Loaded state", "leases", len(state.Leases))
		}

		// Set up dependencies
		clock := &daemon.RealClock{}
		revoker := &daemon.FileRevoker{}
		notifier := &daemon.BeeepNotifier{}
		ipcServer, err := ipc.NewServer(socketPath, secret)
		if err != nil {
			return err
		}

		// Create and run daemon
		d := daemon.NewDaemon(state, statePath, clock, ipcServer, revoker, notifier)
		slog.Info("Daemon startup successful.", "socket", ipcServer.SocketPath())

		return d.Run(context.Background())
	},
}

var cleanupCmd = &cobra.Command{
	Use:   "cleanup",
	Short: "Cleanup orphaned leases.",
	Long:  `Cleanup orphaned leases.`,
	RunE: func(cmd *cobra.Command, args []string) error {
		client := newClient()
		req := ipc.CleanupRequest{Command: "cleanup"}
		var resp ipc.CleanupResponse

		if err := client.Send(req, &resp); err != nil {
			handleClientError(err)
		}

		for _, msg := range resp.Messages {
			fmt.Println(msg)
		}
		return nil
	},
}

func init() {
	daemonCmd.AddCommand(daemonInstallCmd)
	daemonCmd.AddCommand(daemonUninstallCmd)
	daemonCmd.AddCommand(daemonStatusCmd)
	daemonCmd.AddCommand(runCmd)
	daemonCmd.AddCommand(cleanupCmd)
	daemonCmd.AddCommand(daemonReloadCmd)
	rootCmd.AddCommand(daemonCmd)
}
