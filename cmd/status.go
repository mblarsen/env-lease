package cmd

import (
	"fmt"
	"github.com/mblarsen/env-lease/internal/ipc"
	"github.com/spf13/cobra"
	"os"
	"text/tabwriter"
	"time"
)

var statusCmd = &cobra.Command{
	Use:   "status",
	Short: "Show the status of active leases.",
	Long:  `Show the status of active leases.`,
	RunE: func(cmd *cobra.Command, args []string) error {
		secret, err := getSecret()
		if err != nil {
			return fmt.Errorf("failed to get secret: %w", err)
		}
		client := ipc.NewClient(getSocketPath(), secret)
		req := ipc.StatusRequest{Command: "status"}
		var resp ipc.StatusResponse
		if err := client.Send(req, &resp); err != nil {
			return fmt.Errorf("failed to get status: %w", err)
		}

		if len(resp.Leases) == 0 {
			fmt.Println("No active leases.")
			return nil
		}

		showAll, _ := cmd.Flags().GetBool("all")
		if !showAll {
			fmt.Printf("%d active leases. Use --all to show all leases.\n", len(resp.Leases))
			return nil
		}

		w := tabwriter.NewWriter(os.Stdout, 0, 0, 3, ' ', 0)
		fmt.Fprintln(w, "VARIABLE\tSOURCE\tDESTINATION\tEXPIRES IN")
		for _, lease := range resp.Leases {
			expiresIn := time.Until(lease.ExpiresAt).Round(time.Second)
			fmt.Fprintf(w, "%s\t%s\t%s\t%s\n", lease.Variable, lease.Source, lease.Destination, expiresIn)
		}
		return w.Flush()
	},
}

func init() {
	statusCmd.Flags().Bool("all", false, "Show all active leases.")
	rootCmd.AddCommand(statusCmd)
}
