package cmd

import (
	"fmt"
	"github.com/mblarsen/env-lease/internal/config"
	"github.com/mblarsen/env-lease/internal/fileutil"
	"github.com/mblarsen/env-lease/internal/ipc"
	"github.com/spf13/cobra"
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

		leases := make([]ipc.Lease, len(cfg.Lease))
		for i, l := range cfg.Lease {
			leases[i] = ipc.Lease{
				Source:      l.Source,
				Destination: l.Destination,
				Duration:    l.Duration,
				LeaseType:   l.LeaseType,
				Variable:    l.Variable,
				Format:      l.Format,
				Encoding:    l.Encoding,
			}
		}

		req := ipc.GrantRequest{
			Command: "grant",
			Leases:  leases,
		}

		// In a real implementation, we would fetch the secret here.
		// For now, we'll just write a dummy value.
		for _, l := range cfg.Lease {
			if l.LeaseType == "env" {
				content := fmt.Sprintf(l.Format, l.Variable, "dummy-value")
				if err := fileutil.AtomicWriteFile(l.Destination, []byte(content+"\n"), 0644); err != nil {
					return err
				}
			}
		}

		secret, err := getSecret()
		if err != nil {
			return fmt.Errorf("failed to get secret: %w", err)
		}
		client := ipc.NewClient(getSocketPath(), secret)
		if err := client.Send(req, nil); err != nil {
			return fmt.Errorf("failed to send grant request: %w", err)
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
