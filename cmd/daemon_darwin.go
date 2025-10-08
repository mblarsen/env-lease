//go:build darwin
// +build darwin

package cmd

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"

	"github.com/mblarsen/env-lease/internal/fileutil"
	"github.com/spf13/cobra"
)

const (
	daemonServiceName = "com.user.env-lease.plist"
)

func init() {
	daemonInstallCmd.RunE = runInstallDaemon
	daemonUninstallCmd.RunE = runUninstallDaemon
	daemonStatusCmd.RunE = runStatusDaemon
	daemonReloadCmd.RunE = runReloadDaemon
}

func runReloadDaemon(cmd *cobra.Command, args []string) error {
	homeDir, err := os.UserHomeDir()
	if err != nil {
		return err
	}

	plistPath := filepath.Join(homeDir, launchdDir, daemonServiceName)

	// Unload the service
	if err := exec.Command("launchctl", "unload", plistPath).Run(); err != nil {
		return fmt.Errorf("failed to unload launchd service: %w", err)
	}

	// Load the service
	if err := exec.Command("launchctl", "load", plistPath).Run(); err != nil {
		return fmt.Errorf("failed to load launchd service: %w", err)
	}

	fmt.Printf("Successfully reloaded daemon service.\n")
	return nil
}

func runInstallDaemon(cmd *cobra.Command, args []string) error {
	executable, err := os.Executable()
	if err != nil {
		return err
	}

	homeDir, err := os.UserHomeDir()
	if err != nil {
		return err
	}

	// Write the launchd plist
	plistPath := filepath.Join(homeDir, launchdDir, daemonServiceName)
	plistContent := fmt.Sprintf(daemonPlistTemplate, executable, homeDir, homeDir)
	if _, err := fileutil.AtomicWriteFile(plistPath, []byte(plistContent), 0644); err != nil {
		return err
	}

	// Load the service
	if err := exec.Command("launchctl", "load", plistPath).Run(); err != nil {
		return fmt.Errorf("failed to load launchd service: %w", err)
	}

	fmt.Printf("Successfully installed and started daemon service.\n")
	return nil
}

func runUninstallDaemon(cmd *cobra.Command, args []string) error {
	homeDir, err := os.UserHomeDir()
	if err != nil {
		return err
	}

	plistPath := filepath.Join(homeDir, launchdDir, daemonServiceName)

	// Unload the service
	_ = exec.Command("launchctl", "unload", plistPath).Run()

	// Remove files
	_ = os.Remove(plistPath)

	fmt.Println("Successfully uninstalled daemon service.")
	return nil
}

func runStatusDaemon(cmd *cobra.Command, args []string) error {
	homeDir, err := os.UserHomeDir()
	if err != nil {
		return err
	}

	plistPath := filepath.Join(homeDir, launchdDir, daemonServiceName)
	if _, err := os.Stat(plistPath); os.IsNotExist(err) {
		fmt.Println("Daemon service is not installed.")
		return nil
	}

	fmt.Println("Daemon service is installed.")
	fmt.Printf("Configuration file: %s\n", plistPath)
	return nil
}

const daemonPlistTemplate = `<?xml version="1.0" encoding="UTF-8"?>
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
    <key>StandardOutPath</key>
    <string>%s/Library/Logs/env-lease.log</string>
    <key>StandardErrorPath</key>
    <string>%s/Library/Logs/env-lease.error.log</string>
</dict>
</plist>
`
