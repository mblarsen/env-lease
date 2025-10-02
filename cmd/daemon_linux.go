//go:build linux
// +build linux

package cmd

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"

	"github.com/mblarsen/env-lease/internal/fileutil"
	"github.com/spf13/cobra"
)

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
	daemonInstallCmd.RunE = runInstallDaemon
	daemonUninstallCmd.RunE = runUninstallDaemon
	daemonInstallCmd.Flags().Bool("print", false, "Print the service configuration to stdout instead of installing it.")
}

func runInstallDaemon(cmd *cobra.Command, args []string) error {
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
}

func runUninstallDaemon(cmd *cobra.Command, args []string) error {
	servicePath := filepath.Join(os.Getenv("HOME"), ".config", "systemd", "user", "env-lease.service")

	if err := exec.Command("systemctl", "--user", "disable", "--now", "env-lease.service").Run(); err != nil {
		// Ignore errors, as the service may not be running
	}

	if err := os.Remove(servicePath); err != nil {
		return err
	}

	fmt.Println("Successfully uninstalled env-lease daemon service.")
	return nil
}
