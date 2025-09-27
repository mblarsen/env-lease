package cmd

import (
	"fmt"
	"github.com/mblarsen/env-lease/internal/ipc"
	"github.com/spf13/cobra"
	"os"
	"path/filepath"
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

		// First, get the list of active leases from the daemon
		var leasesResp ipc.StatusResponse
		statusReq := ipc.StatusRequest{Command: "status"}
		if err := client.Send(statusReq, &leasesResp); err != nil {
			return fmt.Errorf("failed to get active leases: %w", err)
		}

		// Then, send the revoke request
		req := ipc.RevokeRequest{Command: "revoke"}
		var revokeResp ipc.RevokeResponse
		if err := client.Send(req, &revokeResp); err != nil {
			return fmt.Errorf("failed to send revoke request: %w", err)
		}

		for _, msg := range revokeResp.Messages {
			fmt.Println(msg)
		}

		noDirenv, _ := cmd.Flags().GetBool("no-direnv")
		for _, l := range leasesResp.Leases {
			if filepath.Base(l.Destination) == ".envrc" {
				HandleDirenv(noDirenv, os.Stdout)
				break
			}
		}

		fmt.Println("Revoke request sent.")
		return nil
	},
}

func init() {
	revokeCmd.Flags().Bool("no-direnv", false, "Do not automatically run 'direnv allow'.")
	rootCmd.AddCommand(revokeCmd)
}
