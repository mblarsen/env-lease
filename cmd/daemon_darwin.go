//go:build darwin
// +build darwin

package cmd

import (
	"context"
	"fmt"
	"os"

	"github.com/mblarsen/env-lease/internal/platforminstall"
	"github.com/spf13/cobra"
)

func init() {
	daemonInstallCmd.RunE = runInstallDaemon
	daemonUninstallCmd.RunE = runUninstallDaemon
	daemonStatusCmd.RunE = runStatusDaemon
	daemonReloadCmd.RunE = runReloadDaemon
}

func runReloadDaemon(cmd *cobra.Command, args []string) error {
	installer, err := platforminstall.DefaultManager()
	if err != nil {
		return err
	}
	result, err := installer.ReloadDaemon(context.Background())
	if err != nil {
		return err
	}
	fmt.Println(result.Message)
	return nil
}

func runInstallDaemon(cmd *cobra.Command, args []string) error {
	installer, err := platforminstall.DefaultManager()
	if err != nil {
		return err
	}
	result, err := installer.InstallDaemon(context.Background(), platforminstall.DaemonOptions{})
	if err != nil {
		return err
	}
	fmt.Println(result.Message)
	return nil
}

func runUninstallDaemon(cmd *cobra.Command, args []string) error {
	installer, err := platforminstall.DefaultManager()
	if err != nil {
		return err
	}
	result, err := installer.UninstallDaemon(context.Background())
	if err != nil {
		return err
	}
	fmt.Println(result.Message)
	return nil
}

func runStatusDaemon(cmd *cobra.Command, args []string) error {
	installer, err := platforminstall.DefaultManager()
	if err != nil {
		return err
	}
	status, err := installer.StatusDaemon(context.Background())
	if err != nil {
		return err
	}
	if !status.Installed {
		fmt.Println("Daemon service is not installed.")
		return nil
	}
	fmt.Println("Daemon service is installed.")
	fmt.Fprintf(os.Stdout, "Configuration file: %s\n", status.File)
	return nil
}
