//go:build darwin
// +build darwin

package cmd

import (
	"context"
	"fmt"
	"github.com/mblarsen/env-lease/internal/daemon"
	"github.com/mblarsen/env-lease/internal/fileutil"
	"github.com/mblarsen/env-lease/internal/ipc"
	"github.com/spf13/cobra"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"syscall"
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

		// Remove the quarantine attribute to prevent Gatekeeper issues
		_ = exec.Command("xattr", "-d", "com.apple.quarantine", executable).Run()

		plist := fmt.Sprintf(plistTemplate, executable)
		plistPath := filepath.Join(os.Getenv("HOME"), "Library", "LaunchAgents", "com.user.env-lease.plist")

		if _, err := fileutil.AtomicWriteFile(plistPath, []byte(plist), 0644); err != nil {
			return err
		}

		return exec.Command("launchctl", "load", plistPath).Run()
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

		return os.Remove(plistPath)
	},
}

var runCmd = &cobra.Command{
	Use:   "run",
	Short: "Run the env-lease daemon.",
	Long:  `Run the env-lease daemon.`,
	RunE: func(cmd *cobra.Command, args []string) error {
		fmt.Println("Starting daemon...")

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
			return err
		}

		// Set up dependencies
		clock := &daemon.RealClock{}
		revoker := &daemon.FileRevoker{}
		ipcServer, err := ipc.NewServer(socketPath, secret)
		if err != nil {
			return err
		}

		// Create and run daemon
		d := daemon.NewDaemon(state, statePath, clock, ipcServer, revoker)

		ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
		defer stop()

		go func() {
			<-ctx.Done()
			fmt.Println("Daemon shutting down...")
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
</dict>
</plist>
`

func init() {
	daemonCmd.AddCommand(installCmd)
	daemonCmd.AddCommand(uninstallCmd)
	daemonCmd.AddCommand(runCmd)
	daemonCmd.AddCommand(cleanupCmd)
	rootCmd.AddCommand(daemonCmd)
}
