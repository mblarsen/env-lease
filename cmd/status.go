package cmd

import (
	"fmt"
	"github.com/mblarsen/env-lease/internal/daemon"
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
		client := ipc.NewClient(socketPath, secret)
		req := struct{ Command string }{Command: "status"}
		var resp daemon.State
		if err := client.Send(req, &resp); err != nil {
			return fmt.Errorf("failed to get status: %w", err)
		}

		if len(resp.Leases) == 0 {
			fmt.Println("No active leases.")
			return nil
		}

		w := tabwriter.NewWriter(os.Stdout, 0, 0, 3, ' ', 0)
		fmt.Fprintln(w, "SOURCE\tDESTINATION\tEXPIRES IN")
		for _, lease := range resp.Leases {
			expiresIn := time.Until(lease.ExpiresAt).Round(time.Second)
			fmt.Fprintf(w, "%s\t%s\t%s\n", lease.Source, lease.Destination, expiresIn)
		}
		return w.Flush()
	},
}

func init() {
	rootCmd.AddCommand(statusCmd)
}
