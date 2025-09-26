//go:build linux
// +build linux

package cmd

import (
	"fmt"
	"github.com/mblarsen/env-lease/internal/fileutil"
	"github.com/spf13/cobra"
	"os"
	"os/exec"
	"path/filepath"
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

		service := fmt.Sprintf(serviceTemplate, executable)
		servicePath := filepath.Join(os.Getenv("HOME"), ".config", "systemd", "user", "env-leased.service")

		if _, err := fileutil.AtomicWriteFile(servicePath, []byte(service), 0644); err != nil {
			return err
		}

		return exec.Command("systemctl", "--user", "enable", "--now", "env-leased.service").Run()
	},
}

var uninstallCmd = &cobra.Command{
	Use:   "uninstall",
	Short: "Uninstall the env-lease daemon.",
	Long:  `Uninstall the env-lease daemon.`,
	RunE: func(cmd *cobra.Command, args []string) error {
		servicePath := filepath.Join(os.Getenv("HOME"), ".config", "systemd", "user", "env-leased.service")

		if err := exec.Command("systemctl", "--user", "disable", "--now", "env-leased.service").Run(); err != nil {
			// Ignore errors, as the service may not be running
		}

		return os.Remove(servicePath)
	},
}

const serviceTemplate = `[Unit]
Description=env-lease daemon

[Service]
ExecStart=%s daemon run
Restart=always

[Install]
WantedBy=default.target
`

func init() {
	daemonCmd.AddCommand(installCmd)
	daemonCmd.AddCommand(uninstallCmd)
	rootCmd.AddCommand(daemonCmd)
}
