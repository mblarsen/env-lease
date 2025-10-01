//go:build linux
// +build linux

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
	idleServiceName = "env-lease-idle.service"
	idleTimerName   = "env-lease-idle.timer"
	idleScriptName  = "env-lease-idle-revoke.sh"
	systemdDir      = ".config/systemd/user"
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
	if _, err := time.ParseDuration(checkIntervalStr); err != nil {
		return fmt.Errorf("invalid check-interval duration: %w", err)
	}

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

	// Write the systemd service file
	servicePath := filepath.Join(homeDir, systemdDir, idleServiceName)
	serviceContent := fmt.Sprintf(idleServiceTemplate, scriptPath, timeoutSeconds, executable)
	if _, err := fileutil.AtomicWriteFile(servicePath, []byte(serviceContent), 0644); err != nil {
		return err
	}

	// Write the systemd timer file
	timerPath := filepath.Join(homeDir, systemdDir, idleTimerName)
	timerContent := fmt.Sprintf(timerTemplate, checkIntervalStr, checkIntervalStr)
	if _, err := fileutil.AtomicWriteFile(timerPath, []byte(timerContent), 0644); err != nil {
		return err
	}

	// Enable and start the timer
	if err := exec.Command("systemctl", "--user", "daemon-reload").Run(); err != nil {
		return fmt.Errorf("failed to reload systemd: %w", err)
	}
	if err := exec.Command("systemctl", "--user", "enable", "--now", idleTimerName).Run(); err != nil {
		return fmt.Errorf("failed to enable and start systemd timer: %w", err)
	}

	fmt.Printf("Successfully installed and started idle revocation service. Timeout: %s, Check Interval: %s\n", timeoutStr, checkIntervalStr)
	return nil
}

func runUninstallIdle(cmd *cobra.Command, args []string) error {
	homeDir, err := os.UserHomeDir()
	if err != nil {
		return err
	}

	servicePath := filepath.Join(homeDir, systemdDir, idleServiceName)
	timerPath := filepath.Join(homeDir, systemdDir, idleTimerName)
	scriptPath := filepath.Join(homeDir, scriptDir, idleScriptName)

	// Stop and disable the timer
	_ = exec.Command("systemctl", "--user", "disable", "--now", idleTimerName).Run()

	// Remove files
	_ = os.Remove(servicePath)
	_ = os.Remove(timerPath)
	_ = os.Remove(scriptPath)

	// Reload systemd
	_ = exec.Command("systemctl", "--user", "daemon-reload").Run()

	fmt.Println("Successfully uninstalled idle revocation service.")
	return nil
}

func runStatusIdle(cmd *cobra.Command, args []string) error {
	homeDir, err := os.UserHomeDir()
	if err != nil {
		return err
	}

	timerPath := filepath.Join(homeDir, systemdDir, idleTimerName)
	if _, err := os.Stat(timerPath); os.IsNotExist(err) {
		fmt.Println("Idle revocation service is not installed.")
		return nil
	}

	// Check timer status
	cmdOut, err := exec.Command("systemctl", "--user", "status", idleTimerName).Output()
	if err != nil {
		fmt.Println("Idle revocation service is installed but may not be running.")
	}
	fmt.Println("Idle revocation service status:")
	fmt.Println(string(cmdOut))
	return nil
}

const idleServiceTemplate = `[Unit]
Description=env-lease idle lease revoker

[Service]
Type=oneshot
ExecStart=/bin/sh %s %d %s
`

const timerTemplate = `[Unit]
Description=Run env-lease idle revoker periodically

[Timer]
OnBootSec=%s
OnUnitActiveSec=%s
Unit=env-lease-idle.service

[Install]
WantedBy=timers.target
`
