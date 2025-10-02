package cmd

import (
	"github.com/spf13/cobra"
)

var daemonCmd = &cobra.Command{
	Use:   "daemon",
	Short: "Manage the env-lease daemon.",
	Long:  `Manage the env-lease daemon.`,
}

var daemonInstallCmd = &cobra.Command{
	Use:   "install",
	Short: "Install the env-lease daemon.",
	Long:  `Install the env-lease daemon.`,
}

var daemonUninstallCmd = &cobra.Command{
	Use:   "uninstall",
	Short: "Uninstall the env-lease daemon.",
	Long:  `Uninstall the env-lease daemon.`,
}

var daemonStatusCmd = &cobra.Command{
	Use:   "status",
	Short: "Check the status of the daemon.",
	Long:  `Checks whether the daemon is installed and running.`,
}

func init() {
	daemonCmd.AddCommand(daemonInstallCmd)
	daemonCmd.AddCommand(daemonUninstallCmd)
	daemonCmd.AddCommand(daemonStatusCmd)
	rootCmd.AddCommand(daemonCmd)
}
