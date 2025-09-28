package cmd

import (
	"fmt"
	"github.com/mblarsen/env-lease/internal/config"
	"github.com/mblarsen/env-lease/internal/ipc"
	"github.com/mblarsen/env-lease/internal/provider"
	"github.com/spf13/cobra"
	"log/slog"
	"os"
	"path/filepath"
	"time"
)

var grantCmd = &cobra.Command{
	Use:   "grant",
	Short: "Grant all leases defined in env-lease.toml.",
	Long:  `Grant all leases defined in env-lease.toml.`,
	RunE: func(cmd *cobra.Command, args []string) error {
		cfg, err := config.Load("env-lease.toml")
		if err != nil {
			return fmt.Errorf("failed to load config: %w", err)
		}

		var p provider.SecretProvider
		leases := make([]ipc.Lease, len(cfg.Lease))
		for i, l := range cfg.Lease {
			if os.Getenv("ENV_LEASE_TEST") == "1" {
				p = &provider.MockProvider{}
			} else {
				p = &provider.OnePasswordCLI{
					Account: l.OpAccount,
				}
			}
			duration, err := time.ParseDuration(l.Duration)
			if err != nil {
				return fmt.Errorf("invalid duration '%s': %w", l.Duration, err)
			}
			if duration > 12*time.Hour {
				slog.Warn("Leases longer than 12 hours are discouraged for security reasons.")
			}

			slog.Info("Fetching secret", "source", l.Source)
			secretVal, err := p.Fetch(l.Source)
			if err != nil {
				// TODO: Handle --continue-on-error
				return fmt.Errorf("failed to fetch secret for %s: %w", l.Source, err)
			}
			slog.Info("Fetched secret", "source", l.Source)

			// Write the secret to the destination file.
			override, _ := cmd.Flags().GetBool("override")
			created, err := writeLease(l, secretVal, override)
			if err != nil {
				return fmt.Errorf("failed to write lease for %s: %w", l.Source, err)
			}
			clearString(secretVal)
			if created {
				fmt.Printf("Created file: %s\n", l.Destination)
			}


			absDest, err := filepath.Abs(l.Destination)
			if err != nil {
				return fmt.Errorf("failed to get absolute path for %s: %w", l.Destination, err)
			}

			leases[i] = ipc.Lease{
				Source:      l.Source,
				Destination: absDest,
				Duration:    l.Duration,
				LeaseType:   l.LeaseType,
				Variable:    l.Variable,
				Format:      l.Format,
				Encoding:    l.Encoding,
				FileMode:    l.FileMode,
			}
		}

		configFile, err := filepath.Abs("env-lease.toml")
		if err != nil {
			return fmt.Errorf("failed to get absolute path for env-lease.toml: %w", err)
		}

		override, _ := cmd.Flags().GetBool("override")
		req := ipc.GrantRequest{
			Command:    "grant",
			Leases:     leases,
			Override:   override,
			ConfigFile: configFile,
		}

		client := newClient()
		var resp ipc.GrantResponse
		if err := client.Send(req, &resp); err != nil {
			handleClientError(err)
		}

		for _, msg := range resp.Messages {
			fmt.Println(msg)
		}

		noDirenv, _ := cmd.Flags().GetBool("no-direnv")
		for _, l := range leases {
			if filepath.Base(l.Destination) == ".envrc" {
				HandleDirenv(noDirenv, os.Stdout)
				break
			}
		}

		fmt.Println("Grant request sent successfully.")
		return nil
	},
}




func init() {
	grantCmd.Flags().Bool("override", false, "Override existing values in destination files.")
	grantCmd.Flags().Bool("continue-on-error", false, "Continue granting leases even if one fails.")
	grantCmd.Flags().Bool("no-direnv", false, "Do not automatically run 'direnv allow'.")
	rootCmd.AddCommand(grantCmd)
}
