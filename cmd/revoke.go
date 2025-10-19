package cmd

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"

	"github.com/mblarsen/env-lease/internal/config"
	"github.com/mblarsen/env-lease/internal/ipc"
	"github.com/spf13/cobra"
)

var revokeCmd = &cobra.Command{
	Use:   "revoke",
	Short: "Revoke all active leases for the current project.",
	Long:  `Revoke all active leases for the current project.`,
	RunE: func(cmd *cobra.Command, args []string) error {
		resetConfirmState()
		client := newClient()

		configFileFlag, _ := cmd.Flags().GetString("config")
		absConfigFile, err := config.ResolveConfigFile(configFileFlag)
		if err != nil {
			return err
		}

		all, _ := cmd.Flags().GetBool("all")
		interactive, _ := cmd.Flags().GetBool("interactive")

		if client == nil {
			fmt.Println("Revoke command running in test mode.")
			return nil
		}
		if interactive {
			statusReq := ipc.StatusRequest{Command: "status"}
			if !all {
				statusReq.ConfigFile = absConfigFile
			}
			var leasesResp ipc.StatusResponse
			if err := client.Send(statusReq, &leasesResp); err != nil {
				handleClientError(err)
			}

			var leasesToRevoke []ipc.Lease
			childrenOfParent := make(map[string][]ipc.Lease)
			var potentialParentsAndNormalLeases []ipc.Lease

			// First pass: separate children from potential parents/normal leases
			for _, l := range leasesResp.Leases {
				if l.ParentSource != "" {
					childrenOfParent[l.ParentSource] = append(childrenOfParent[l.ParentSource], l)
				} else {
					potentialParentsAndNormalLeases = append(potentialParentsAndNormalLeases, l)
				}
			}

			var parents []ipc.Lease
			var normalLeases []ipc.Lease

			// Second pass: separate actual parents from normal leases
			for _, l := range potentialParentsAndNormalLeases {
				uniqueID := l.Source + "->" + l.Destination
				if _, isParent := childrenOfParent[uniqueID]; isParent {
					parents = append(parents, l)
				} else {
					normalLeases = append(normalLeases, l)
				}
			}

			// Process parents and their children
			for _, p := range parents {
				uniqueID := p.Source + "->" + p.Destination
				children := childrenOfParent[uniqueID]
				sort.Slice(children, func(i, j int) bool {
					return children[i].Variable < children[j].Variable
				})
				var childrenToRevoke []ipc.Lease

				for _, child := range children {
					if confirm(fmt.Sprintf("Revoke lease for '%s'?", child.Variable)) {
						childrenToRevoke = append(childrenToRevoke, child)
					}
				}

				// If all children are being revoked, add the parent to the list too
				if len(childrenToRevoke) == len(children) {
					leasesToRevoke = append(leasesToRevoke, p)
				}
				leasesToRevoke = append(leasesToRevoke, childrenToRevoke...)
			}

			// Process normal leases
			sort.Slice(normalLeases, func(i, j int) bool {
				return normalLeases[i].Variable < normalLeases[j].Variable
			})
			for _, l := range normalLeases {
				promptVar := l.Source
				if l.Variable != "" {
					promptVar = l.Variable
				}
				if confirm(fmt.Sprintf("Revoke lease for '%s'?", promptVar)) {
					leasesToRevoke = append(leasesToRevoke, l)
				}
			}

			if len(leasesToRevoke) == 0 {
				fmt.Println("No leases selected for revocation.")
				return nil
			}

			req := ipc.RevokeRequest{
				Command:    "revoke",
				ConfigFile: absConfigFile,
				Leases:     leasesToRevoke,
			}
			var revokeResp ipc.RevokeResponse
			if err := client.Send(req, &revokeResp); err != nil {
				handleClientError(err)
			}
			for _, msg := range revokeResp.Messages {
				fmt.Println(msg)
			}
			fmt.Println("Revoke request sent.")
			return nil
		}

		req := ipc.RevokeRequest{
			Command:    "revoke",
			ConfigFile: absConfigFile,
			All:        all,
		}
		var revokeResp ipc.RevokeResponse
		if err := client.Send(req, &revokeResp); err != nil {
			handleClientError(err)
		}

		isShellMode := len(revokeResp.ShellCommands) > 0

		for _, msg := range revokeResp.Messages {
			if isShellMode {
				fmt.Fprintln(os.Stderr, msg)
			} else {
				fmt.Println(msg)
			}
		}

		if isShellMode {
			fmt.Println("# When using shell lease types run this command like `eval $(env-lease revoke)`")
			for _, shellCmd := range revokeResp.ShellCommands {
				fmt.Println(shellCmd)
			}
		}

		// If all leases were revoked, check for .envrc and handle direnv
		var leasesResp ipc.StatusResponse
		statusReq := ipc.StatusRequest{Command: "status"}
		if err := client.Send(statusReq, &leasesResp); err != nil {
			// If we can't get the status, we can't check for .envrc, so we'll just print the message and return.
			handleClientError(err)
		} else {
			noDirenv, _ := cmd.Flags().GetBool("no-direnv")
			for _, l := range leasesResp.Leases {
				if filepath.Base(l.Destination) == ".envrc" {
					writer := os.Stdout
					if isShellMode {
						writer = os.Stderr
					}
					HandleDirenv(noDirenv, writer)
					break
				}
			}
		}

		finalMsg := "Revoke request sent."
		if isShellMode {
			fmt.Fprintln(os.Stderr, finalMsg)
		} else {
			fmt.Println(finalMsg)
		}

		return nil
	},
}

func init() {
	revokeCmd.Flags().Bool("no-direnv", false, "Do not automatically run 'direnv allow'.")
	revokeCmd.Flags().Bool("all", false, "Revoke all active leases, across all projects.")
	revokeCmd.Flags().BoolP("interactive", "i", false, "Prompt for confirmation before revoking each lease.")
	revokeCmd.Flags().StringP("config", "c", "env-lease.toml", "Path to config file.")
	revokeCmd.Flags().String("local-config", "", "Path to local override config file.")
	rootCmd.AddCommand(revokeCmd)
}
