//go:build darwin
// +build darwin

package cmd

import (
	"context"
	"fmt"
	"github.com/lmittmann/tint"
	"github.com/mblarsen/env-lease/internal/daemon"
	"github.com/mblarsen/env-lease/internal/fileutil"
	"github.com/mblarsen/env-lease/internal/ipc"
	"github.com/spf13/cobra"
	"log/slog"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"syscall"
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

		plist := fmt.Sprintf(plistTemplate, executable)
		if print, _ := cmd.Flags().GetBool("print"); print {
			fmt.Fprint(os.Stdout, plist)
			fmt.Fprintln(os.Stderr, "WARNING: Service configuration printed but not installed.")
			return nil
		}
		plistPath := filepath.Join(os.Getenv("HOME"), "Library", "LaunchAgents", "com.user.env-lease.plist")

		if _, err := fileutil.AtomicWriteFile(plistPath, []byte(plist), 0644); err != nil {
			return err
		}

		if err := exec.Command("launchctl", "load", plistPath).Run(); err != nil {
			return err
		}

		fmt.Printf("Successfully installed env-lease daemon service. Configuration file created at: %s\n", plistPath)
		return nil
	},
}

var uninstallCmd = &cobra.Command{
	Use:   "uninstall",
	Short: "Uninstall the env-lease daemon.",
	Long:  `Uninstall the env-lease daemon.`,
	RunE: func(cmd *cobra.Command, args []string) error {
		plistPath := filepath.Join(os.Getenv("HOME"), "Library", "LaunchAgents", "com.user.env-lease.plist")

		if err := exec.Command("launchctl", "unload", plistPath).Run(); err != nil {
			// Ignore errors, as the service may not be loaded
		}

		if err := os.Remove(plistPath); err != nil {
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
		slog.SetDefault(slog.New(
			tint.NewHandler(os.Stderr, &tint.Options{
				Level:      slog.LevelDebug,
				TimeFormat: time.Kitchen,
			}),
		))
		slog.Info("Starting daemon...")

		// Configuration paths
		configDir := filepath.Join(os.Getenv("HOME"), ".config", "env-lease")
		if err := os.MkdirAll(configDir, 0700); err != nil {
			return err
		}
		socketPath := filepath.Join(configDir, "daemon.sock")
		statePath := filepath.Join(configDir, "state.json")
		secretPath := filepath.Join(configDir, "auth.token")

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
		notifier := &daemon.OsaScriptNotifier{}
		ipcServer, err := ipc.NewServer(socketPath, secret)
		if err != nil {
			return err
		}

		// Create and run daemon
		d := daemon.NewDaemon(state, statePath, clock, ipcServer, revoker, notifier)
		slog.Info("Daemon startup successful.", "socket", ipcServer.SocketPath())


		ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
		defer stop()

		go func() {
			<-ctx.Done()
			slog.Info("Daemon shutting down...")
			state.SaveState(statePath)
		}()

		return d.Run(ctx)
	},
}

var cleanupCmd = &cobra.Command{
	Use:   "cleanup",
	Short: "Cleanup orphaned leases.",
	Long:  `Cleanup orphaned leases.`,
	RunE: func(cmd *cobra.Command, args []string) error {
		fmt.Println("Cleaning up orphaned leases...")
		// This is where the cleanup will be triggered
		return nil
	},
}

var reloadCmd = &cobra.Command{
	Use:   "reload",
	Short: "Reload the env-lease daemon.",
	Long:  `Reload the env-lease daemon.`,
	RunE: func(cmd *cobra.Command, args []string) error {
		plistPath := filepath.Join(os.Getenv("HOME"), "Library", "LaunchAgents", "com.user.env-lease.plist")

		// If the service file doesn't exist, do nothing.
		if _, err := os.Stat(plistPath); os.IsNotExist(err) {
			fmt.Println("Daemon service not installed, nothing to do.")
			return nil
		}

		// Unload the service, ignoring errors in case it's not loaded.
		_ = exec.Command("launchctl", "unload", plistPath).Run()

		// Load the service.
		if err := exec.Command("launchctl", "load", plistPath).Run(); err != nil {
			return err
		}

		fmt.Println("Successfully reloaded env-lease daemon service.")
		return nil
	},
}

const plistTemplate = `<?xml version="1.0" encoding="UTF-8"?>
<!DOCTYPE plist PUBLIC "-//Apple//DTD PLIST 1.0//EN" "http://www.apple.com/DTDs/PropertyList-1.0.dtd">
<plist version="1.0">
<dict>
	<key>Label</key>
	<string>com.user.env-lease</string>
	<key>ProgramArguments</key>
	<array>
		<string>%s</string>
		<string>daemon</string>
		<string>run</string>
	</array>
	<key>RunAtLoad</key>
	<true/>
	<key>KeepAlive</key>
	<true/>
	<key>EnvironmentVariables</key>
	<dict>
		<key>ENV_LEASE_LOG_LEVEL</key>
		<string>info</string>
	</dict>
</dict>
</plist>
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
