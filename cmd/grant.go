package cmd

import (
	"fmt"
	"github.com/mblarsen/env-lease/internal/config"
	"github.com/mblarsen/env-lease/internal/ipc"
	"github.com/mblarsen/env-lease/internal/provider"
	"github.com/spf13/cobra"
	"os"
	"path/filepath"
	"strings"
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
		if os.Getenv("ENV_LEASE_TEST") == "1" {
			p = &provider.MockProvider{}
		} else {
			p = &provider.OnePasswordCLI{}
		}

		leases := make([]ipc.Lease, len(cfg.Lease))
		for i, l := range cfg.Lease {
			duration, err := time.ParseDuration(l.Duration)
			if err != nil {
				return fmt.Errorf("invalid duration '%s': %w", l.Duration, err)
			}
			if duration > 12*time.Hour {
				fmt.Fprintf(os.Stderr, "Warning: Leases longer than 12 hours are discouraged for security reasons.\n")
			}

			secretVal, err := p.Fetch(l.Source)
			if err != nil {
				// TODO: Handle --continue-on-error
				return fmt.Errorf("failed to fetch secret for %s: %w", l.Source, err)
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
				Value:       strings.TrimSpace(secretVal),
				FileMode:    l.FileMode,
			}
		}

		override, _ := cmd.Flags().GetBool("override")
		req := ipc.GrantRequest{
			Command:  "grant",
			Leases:   leases,
			Override: override,
		}

		ipcSecret, err := getSecret()
		if err != nil {
			return fmt.Errorf("failed to get ipc secret: %w", err)
		}
		client := ipc.NewClient(getSocketPath(), ipcSecret)
		var resp ipc.GrantResponse
		if err := client.Send(req, &resp); err != nil {
			return fmt.Errorf("failed to send grant request: %w", err)
		}

		for _, msg := range resp.Messages {
			fmt.Println(msg)
		}

		fmt.Println("Grant request sent successfully.")
		return nil
	},
}

func init() {
	grantCmd.Flags().Bool("override", false, "Override existing values in destination files.")
	grantCmd.Flags().Bool("continue-on-error", false, "Continue granting leases even if one fails.")
	rootCmd.AddCommand(grantCmd)
}
