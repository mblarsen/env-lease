//go:build darwin
// +build darwin

package cmd

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"time"

	"github.com/mblarsen/env-lease/internal/fileutil"
	"github.com/spf13/cobra"
)

const (
	idleServiceName = "com.user.env-lease-idle.plist"
	idleScriptName  = "env-lease-idle-revoke.sh"
	scriptDir       = ".local/bin"
)

func init() {
	idleInstallCmd.RunE = runInstallIdle
	idleUninstallCmd.RunE = runUninstallIdle
	idleStatusCmd.RunE = runStatusIdle
}

func runInstallIdle(cmd *cobra.Command, args []string) error {
	timeoutStr, _ := cmd.Flags().GetString("timeout")
	duration, err := time.ParseDuration(timeoutStr)
	if err != nil {
		return fmt.Errorf("invalid timeout duration: %w", err)
	}
	timeoutSeconds := int(duration.Seconds())

	checkIntervalStr, _ := cmd.Flags().GetString("check-interval")
	checkInterval, err := time.ParseDuration(checkIntervalStr)
	if err != nil {
		return fmt.Errorf("invalid check-interval duration: %w", err)
	}
	checkIntervalSeconds := int(checkInterval.Seconds())

	executable, err := os.Executable()
	if err != nil {
		return err
	}

	homeDir, err := os.UserHomeDir()
	if err != nil {
		return err
	}

	// Write the script
	scriptPath := filepath.Join(homeDir, scriptDir, idleScriptName)
	if err := os.MkdirAll(filepath.Dir(scriptPath), 0755); err != nil {
		return err
	}
	if _, err := fileutil.AtomicWriteFile(scriptPath, []byte(idleRevokeScript), 0755); err != nil {
		return err
	}

	// Write the launchd plist
	plistPath := filepath.Join(homeDir, launchdDir, idleServiceName)
	plistContent := fmt.Sprintf(idlePlistTemplate, scriptPath, timeoutSeconds, executable, checkIntervalSeconds, homeDir, homeDir)
	if _, err := fileutil.AtomicWriteFile(plistPath, []byte(plistContent), 0644); err != nil {
		return err
	}

	// Load the service
	if err := exec.Command("launchctl", "load", plistPath).Run(); err != nil {
		return fmt.Errorf("failed to load launchd service: %w", err)
	}

	fmt.Printf("Successfully installed and started idle revocation service. Timeout: %s, Check Interval: %s\n", timeoutStr, checkIntervalStr)
	return nil
}

func runUninstallIdle(cmd *cobra.Command, args []string) error {
	homeDir, err := os.UserHomeDir()
	if err != nil {
		return err
	}

	plistPath := filepath.Join(homeDir, launchdDir, idleServiceName)
	scriptPath := filepath.Join(homeDir, scriptDir, idleScriptName)

	// Unload the service
	_ = exec.Command("launchctl", "unload", plistPath).Run()

	// Remove files
	_ = os.Remove(plistPath)
	_ = os.Remove(scriptPath)

	fmt.Println("Successfully uninstalled idle revocation service.")
	return nil
}

func runStatusIdle(cmd *cobra.Command, args []string) error {
	homeDir, err := os.UserHomeDir()
	if err != nil {
		return err
	}

	plistPath := filepath.Join(homeDir, launchdDir, idleServiceName)
	if _, err := os.Stat(plistPath); os.IsNotExist(err) {
		fmt.Println("Idle revocation service is not installed.")
		return nil
	}

	// Check if the service is loaded
	// Note: This is a best-effort check. `launchctl list` is not easily scriptable.
	fmt.Println("Idle revocation service is installed.")
	fmt.Printf("Configuration file: %s\n", plistPath)
	return nil
}

const idlePlistTemplate = `<?xml version="1.0" encoding="UTF-8"?>
<!DOCTYPE plist PUBLIC "-//Apple//DTD PLIST 1.0//EN" "http://www.apple.com/DTDs/PropertyList-1.0.dtd">
<plist version="1.0">
<dict>
    <key>Label</key>
    <string>com.user.env-lease-idle</string>
    <key>ProgramArguments</key>
    <array>
        <string>/bin/sh</string>
        <string>%s</string>
        <string>%d</string>
        <string>%s</string>
    </array>
    <key>RunAtLoad</key>
    <true/>
    <key>StartInterval</key>
    <integer>%d</integer> <!-- Run every X seconds -->
    <key>StandardOutPath</key>
    <string>%s/Library/Logs/env-lease-idle.log</string>
    <key>StandardErrorPath</key>
    <string>%s/Library/Logs/env-lease-idle.error.log</string>
</dict>
</plist>
`
