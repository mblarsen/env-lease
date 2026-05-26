//go:build darwin
// +build darwin

package cmd

import (
	"context"
	"fmt"
	"os"
	"time"

	"github.com/mblarsen/env-lease/internal/platforminstall"
	"github.com/spf13/cobra"
)

func init() {
	idleInstallCmd.RunE = runInstallIdle
	idleUninstallCmd.RunE = runUninstallIdle
	idleStatusCmd.RunE = runStatusIdle
}

func runInstallIdle(cmd *cobra.Command, args []string) error {
	opts, err := idleOptionsFromFlags(cmd)
	if err != nil {
		return err
	}
	installer, err := platforminstall.DefaultManager()
	if err != nil {
		return err
	}
	result, err := installer.InstallIdle(context.Background(), opts)
	if err != nil {
		return err
	}
	fmt.Println(result.Message)
	return nil
}

func runUninstallIdle(cmd *cobra.Command, args []string) error {
	installer, err := platforminstall.DefaultManager()
	if err != nil {
		return err
	}
	result, err := installer.UninstallIdle(context.Background())
	if err != nil {
		return err
	}
	fmt.Println(result.Message)
	return nil
}

func runStatusIdle(cmd *cobra.Command, args []string) error {
	installer, err := platforminstall.DefaultManager()
	if err != nil {
		return err
	}
	status, err := installer.StatusIdle(context.Background())
	if err != nil {
		return err
	}
	if !status.Installed {
		fmt.Println("Idle revocation service is not installed.")
		return nil
	}
	fmt.Println("Idle revocation service is installed.")
	fmt.Fprintf(os.Stdout, "Configuration file: %s\n", status.File)
	return nil
}

func idleOptionsFromFlags(cmd *cobra.Command) (platforminstall.IdleOptions, error) {
	timeoutStr, _ := cmd.Flags().GetString("timeout")
	timeout, err := time.ParseDuration(timeoutStr)
	if err != nil {
		return platforminstall.IdleOptions{}, fmt.Errorf("invalid timeout duration: %w", err)
	}

	checkIntervalStr, _ := cmd.Flags().GetString("check-interval")
	checkInterval, err := time.ParseDuration(checkIntervalStr)
	if err != nil {
		return platforminstall.IdleOptions{}, fmt.Errorf("invalid check-interval duration: %w", err)
	}

	return platforminstall.IdleOptions{
		Timeout:           timeout,
		CheckInterval:     checkInterval,
		TimeoutText:       timeoutStr,
		CheckIntervalText: checkIntervalStr,
	}, nil
}
