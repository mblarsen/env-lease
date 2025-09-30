package cmd

import (
	"fmt"
	"github.com/mblarsen/env-lease/internal/ipc"
	"github.com/spf13/cobra"
	"os"
	"path/filepath"
	"sort"
	"text/tabwriter"
	"time"
)

var statusCmd = &cobra.Command{
	Use:   "status",
	Short: "Show the status of active leases.",
	Long:  `Show the status of active leases.`,
	RunE: func(cmd *cobra.Command, args []string) error {
		client := newClient()
		req := ipc.StatusRequest{Command: "status"}
		var resp ipc.StatusResponse
		if err := client.Send(req, &resp); err != nil {
			handleClientError(err)
		}

		if len(resp.Leases) == 0 {
			fmt.Println("No active leases.")
			return nil
		}

		configFile, err := filepath.Abs("env-lease.toml")
		if err != nil {
			return fmt.Errorf("failed to get absolute path for env-lease.toml: %w", err)
		}

		showAll, _ := cmd.Flags().GetBool("all")

		// Group all leases hierarchically first
		groupedLeases := make(map[string][]ipc.Lease)
		var allTopLevelLeases []ipc.Lease
		for _, lease := range resp.Leases {
			if lease.ParentSource != "" {
				groupedLeases[lease.ParentSource] = append(groupedLeases[lease.ParentSource], lease)
			} else {
				allTopLevelLeases = append(allTopLevelLeases, lease)
			}
		}

		// Determine which leases to display
		var leasesToDisplay []ipc.Lease
		if showAll {
			leasesToDisplay = allTopLevelLeases
		} else {
			for _, lease := range allTopLevelLeases {
				if lease.ConfigFile == configFile {
					leasesToDisplay = append(leasesToDisplay, lease)
				}
			}
		}

		if len(leasesToDisplay) == 0 {
			fmt.Println("No active leases for this project.")
		} else {
			// Sort top-level leases by destination
			sort.Slice(leasesToDisplay, func(i, j int) bool {
				return leasesToDisplay[i].Destination < leasesToDisplay[j].Destination
			})
			printLeases(leasesToDisplay, groupedLeases)
		}

		// Calculate other leases count
		if !showAll {
			otherLeasesCount := len(allTopLevelLeases) - len(leasesToDisplay)
			if otherLeasesCount > 0 {
				fmt.Println("-------------------------------------------------------")
				fmt.Printf("%d more active leases. Use --all to show all leases.\n", otherLeasesCount)
			}
		}

		return nil
	},
}

func printLeases(leases []ipc.Lease, children map[string][]ipc.Lease) {
	w := tabwriter.NewWriter(os.Stdout, 0, 0, 3, ' ', 0)
	fmt.Fprintln(w, "VARIABLE\tSOURCE\tDESTINATION\tEXPIRES IN")

	for _, lease := range leases {
		expiresIn := time.Until(lease.ExpiresAt).Round(time.Second)
		variable := lease.Variable
		if variable == "" {
			variable = "<exploded>"
		}
		fmt.Fprintf(w, "%s\t%s\t%s\t%s\n", variable, lease.Source, lease.Destination, expiresIn)

		// Create a unique ID for the parent to find children
		uniqueParentID := lease.Source + "->" + lease.Destination
		if childLeases, ok := children[uniqueParentID]; ok {
			// Sort children by variable name
			sort.Slice(childLeases, func(i, j int) bool {
				return childLeases[i].Variable < childLeases[j].Variable
			})
			for i, child := range childLeases {
				expiresInChild := time.Until(child.ExpiresAt).Round(time.Second)
				connector := "├─"
				if i == len(childLeases)-1 {
					connector = "└─"
				}
				fmt.Fprintf(w, " %s %s\t\t%s\t%s\n", connector, child.Variable, child.Destination, expiresInChild)
			}
		}
	}
	w.Flush()
}

func init() {
	statusCmd.Flags().Bool("all", false, "Show all active leases.")
	rootCmd.AddCommand(statusCmd)
}
