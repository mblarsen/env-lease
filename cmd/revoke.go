package cmd

import (
	"fmt"
	"github.com/mblarsen/env-lease/internal/ipc"
	"github.com/spf13/cobra"
)

var revokeCmd = &cobra.Command{
	Use:   "revoke",
	Short: "Revoke all active leases for the current project.",
	Long:  `Revoke all active leases for the current project.`,
	RunE: func(cmd *cobra.Command, args []string) error {
		secret, err := getSecret()
		if err != nil {
			return fmt.Errorf("failed to get secret: %w", err)
		}
		client := ipc.NewClient(getSocketPath(), secret)
		req := struct{ Command string }{Command: "revoke"}
		if err := client.Send(req, nil); err != nil {
			return fmt.Errorf("failed to send revoke request: %w", err)
		}
		fmt.Println("Revoke request sent.")
		return nil
	},
}

func init() {
	rootCmd.AddCommand(revokeCmd)
}
