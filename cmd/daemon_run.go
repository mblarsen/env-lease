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

func init() {
	daemonCmd.AddCommand(runCmd)
}
