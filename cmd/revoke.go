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

		configFile, err := filepath.Abs("env-lease.toml")
		if err != nil {
			return fmt.Errorf("failed to get absolute path for env-lease.toml: %w", err)
		}

		req := ipc.RevokeRequest{
			Command:    "revoke",
			ConfigFile: configFile,
		}
		var revokeResp ipc.RevokeResponse
		if err := client.Send(req, &revokeResp); err != nil {
			return fmt.Errorf("failed to send revoke request: %w", err)
		}

		for _, msg := range revokeResp.Messages {
			fmt.Println(msg)
		}

		// If all leases were revoked, check for .envrc and handle direnv
		var leasesResp ipc.StatusResponse
		statusReq := ipc.StatusRequest{Command: "status"}
		if err := client.Send(statusReq, &leasesResp); err != nil {
			// If we can't get the status, we can't check for .envrc, so we'll just print the message and return.
		} else {
			noDirenv, _ := cmd.Flags().GetBool("no-direnv")
			for _, l := range leasesResp.Leases {
				if filepath.Base(l.Destination) == ".envrc" {
					HandleDirenv(noDirenv, os.Stdout)
					break
				}
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
