package cmd

import (
	"fmt"
	"github.com/spf13/cobra"
)

var runCmd = &cobra.Command{
	Use:   "run",
	Short: "Run the env-lease daemon.",
	Long:  `Run the env-lease daemon.`,
	RunE: func(cmd *cobra.Command, args []string) error {
		fmt.Println("Running daemon...")
		// This is where the daemon will be started
		return nil
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

func init() {
	daemonCmd.AddCommand(runCmd)
	daemonCmd.AddCommand(cleanupCmd)
}
