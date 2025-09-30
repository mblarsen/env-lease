package cmd

import (
	"fmt"
	"github.com/spf13/cobra"
)

var idleCmd = &cobra.Command{
	Use:   "idle",
	Short: "Manage automatic lease revocation on system idle.",
	Long:  `Manage the idle-based lease revocation service.`,
}

var idleInstallCmd = &cobra.Command{
	Use:   "install",
	Short: "Install and start the idle revocation service.",
	Long:  `Installs and starts a system service that automatically revokes all leases after a specified period of user inactivity.`,
	RunE: func(cmd *cobra.Command, args []string) error {
		// This will be implemented in the next steps.
		fmt.Println("Installing idle service...")
		return nil
	},
}

var idleUninstallCmd = &cobra.Command{
	Use:   "uninstall",
	Short: "Uninstall the idle revocation service.",
	Long:  `Stops and removes the idle-based lease revocation service.`,
	RunE: func(cmd *cobra.Command, args []string) error {
		// This will be implemented in the next steps.
		fmt.Println("Uninstalling idle service...")
		return nil
	},
}

var idleStatusCmd = &cobra.Command{
	Use:   "status",
	Short: "Check the status of the idle revocation service.",
	Long:  `Checks whether the idle revocation service is installed and running.`,
	RunE: func(cmd *cobra.Command, args []string) error {
		// This will be implemented in the next steps.
		fmt.Println("Checking idle service status...")
		return nil
	},
}

func init() {
	idleInstallCmd.Flags().String("timeout", "1h", "Set the idle duration before leases are revoked (e.g., '1h', '30m').")
	idleInstallCmd.Flags().String("check-interval", "5m", "Set the frequency for checking idle time (e.g., '1m', '5m').")

	idleCmd.AddCommand(idleInstallCmd)
	idleCmd.AddCommand(idleUninstallCmd)
	idleCmd.AddCommand(idleStatusCmd)

	rootCmd.AddCommand(idleCmd)
}
