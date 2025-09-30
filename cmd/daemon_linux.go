//go:build linux
// +build linux

package cmd

import (
	"context"
	"fmt"
	"github.com/lmittmann/tint"
	"github.com/mblarsen/env-lease/internal/daemon"
	"github.com/mblarsen/env-lease/internal/fileutil"
	"github.com/mblarsen/env-lease/internal/ipc"
	"github.com/mblarsen/env-lease/internal/xdgpath"
	"github.com/spf13/cobra"
	"log/slog"
	"os"
	"os/exec"
	"path/filepath"
	"time"
)

var daemonCmd = &cobra.Command{
	Use:   "daemon",
	Short: "Manage the env-lease daemon.",
	Long:  `Manage the env-lease daemon.`,
}

var installCmd = &cobra.Command{
	Use:   "install",
	Short: "Install the env-lease daemon.",
	Long:  `Install the env-lease daemon.`,
	RunE: func(cmd *cobra.Command, args []string) error {
		executable, err := os.Executable()
		if err != nil {
			return err
		}

		service := fmt.Sprintf(daemonServiceTemplate, executable)
		if print, _ := cmd.Flags().GetBool("print"); print {
			fmt.Fprint(os.Stdout, service)
			fmt.Fprintln(os.Stderr, "WARNING: Service configuration printed but not installed.")
			return nil
		}
		servicePath := filepath.Join(os.Getenv("HOME"), ".config", "systemd", "user", "env-lease.service")

		if _, err := fileutil.AtomicWriteFile(servicePath, []byte(service), 0644); err != nil {
			return err
		}

		if err := exec.Command("systemctl", "--user", "enable", "--now", "env-lease.service").Run(); err != nil {
			return err
		}

		fmt.Printf("Successfully installed env-lease daemon service. Configuration file created at: %s\n", servicePath)
		return nil
	},
}

var uninstallCmd = &cobra.Command{
	Use:   "uninstall",
	Short: "Uninstall the env-lease daemon.",
	Long:  `Uninstall the env-lease daemon.`,
	RunE: func(cmd *cobra.Command, args []string) error {
		servicePath := filepath.Join(os.Getenv("HOME"), ".config", "systemd", "user", "env-lease.service")

		if err := exec.Command("systemctl", "--user", "disable", "--now", "env-lease.service").Run(); err != nil {
			// Ignore errors, as the service may not be running
		}

		if err := os.Remove(servicePath); err != nil {
			return err
		}

		fmt.Println("Successfully uninstalled env-lease daemon service.")
		return nil
	},
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
		notifier := &daemon.NotifySendNotifier{}
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

var reloadCmd = &cobra.Command{
	Use:   "reload",
	Short: "Reload the env-lease daemon.",
	Long:  `Reload the env-lease daemon.`,
	RunE: func(cmd *cobra.Command, args []string) error {
		servicePath := filepath.Join(os.Getenv("HOME"), ".config", "systemd", "user", "env-lease.service")

		// If the service file doesn't exist, do nothing.
		if _, err := os.Stat(servicePath); os.IsNotExist(err) {
			fmt.Println("Daemon service not installed, nothing to do.")
			return nil
		}

		if err := exec.Command("systemctl", "--user", "restart", "env-lease.service").Run(); err != nil {
			return err
		}

		fmt.Println("Successfully reloaded env-lease daemon service.")
		return nil
	},
}

const daemonServiceTemplate = `[Unit]
Description=env-lease daemon

[Service]
ExecStart=%s daemon run
Restart=always
Environment="ENV_LEASE_LOG_LEVEL=info"

[Install]
WantedBy=default.target
`

func init() {
	installCmd.Flags().Bool("print", false, "Print the service configuration to stdout instead of installing it.")
	daemonCmd.AddCommand(installCmd)
	daemonCmd.AddCommand(uninstallCmd)
	daemonCmd.AddCommand(reloadCmd)
	daemonCmd.AddCommand(runCmd)
	daemonCmd.AddCommand(cleanupCmd)
	rootCmd.AddCommand(daemonCmd)
}
