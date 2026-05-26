package cmd

import (
	"fmt"
	"os"

	"github.com/mblarsen/env-lease/internal/config"
	"github.com/mblarsen/env-lease/internal/ipc"
	"github.com/mblarsen/env-lease/internal/lease"
	"github.com/mblarsen/env-lease/internal/presentation"
	"github.com/spf13/cobra"
)

var statusCmd = &cobra.Command{
	Use:   "status",
	Short: "Show the status of active leases.",
	Long:  `Show the status of active leases.`,
	RunE: func(cmd *cobra.Command, args []string) error {
		client := newIPCClient()
		if client == nil {
			fmt.Println("Status command running in test mode.")
			return nil
		}

		req := ipc.StatusRequest{}
		var resp ipc.StatusResponse
		if err := client.Send(req, &resp); err != nil {
			handleClientError(err)
		}

		if len(resp.Leases) == 0 {
			presenter.Print(os.Stdout, presentation.MessageNoActiveLeases)
			return nil
		}

		configFileFlag, _ := cmd.Flags().GetString("config")
		absConfigFile, err := config.ResolveConfigFile(configFileFlag)
		if err != nil {
			return err
		}

		// Status filters daemon state by config path without loading the Config.
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
				if lease.ConfigFile == absConfigFile {
					leasesToDisplay = append(leasesToDisplay, lease)
				}
			}
		}

		if len(leasesToDisplay) == 0 {
			presenter.Print(os.Stdout, presentation.MessageNoActiveLeasesForProject)
		} else {
			presenter.RenderStatus(statusPresentationLeases(leasesToDisplay, groupedLeases), os.Stdout)
		}

		// Calculate other leases count, excluding parent leases from the count
		if !showAll {
			var allLeasesCount int
			for _, topLevel := range allTopLevelLeases {
				parentID := lease.ParentIdentity(topLevel.Source, topLevel.Destination)
				if children, isParent := groupedLeases[parentID]; isParent {
					allLeasesCount += len(children)
				} else {
					allLeasesCount++
				}
			}

			var displayedLeasesCount int
			for _, displayed := range leasesToDisplay {
				parentID := lease.ParentIdentity(displayed.Source, displayed.Destination)
				if children, isParent := groupedLeases[parentID]; isParent {
					displayedLeasesCount += len(children)
				} else {
					displayedLeasesCount++
				}
			}

			otherLeasesCount := allLeasesCount - displayedLeasesCount
			if otherLeasesCount > 0 {
				presenter.Print(os.Stdout, presentation.MessageOtherActiveLeases, otherLeasesCount)
			}
		}

		return nil
	},
}

func statusPresentationLeases(topLevel []ipc.Lease, children map[string][]ipc.Lease) []presentation.StatusLease {
	leases := make([]presentation.StatusLease, 0, len(topLevel))
	for _, displayed := range topLevel {
		leases = append(leases, statusPresentationLease(displayed))
		parentID := lease.ParentIdentity(displayed.Source, displayed.Destination)
		for _, child := range children[parentID] {
			leases = append(leases, statusPresentationLease(child))
		}
	}
	return leases
}

func statusPresentationLease(l ipc.Lease) presentation.StatusLease {
	return presentation.StatusLease{
		ID:          lease.ParentIdentity(l.Source, l.Destination),
		ParentID:    l.ParentSource,
		Variable:    l.Variable,
		Source:      l.Source,
		Destination: l.Destination,
		LeaseType:   l.LeaseType,
		ExpiresAt:   l.ExpiresAt,
	}
}

func init() {
	statusCmd.Flags().Bool("all", false, "Show all active leases.")
	statusCmd.Flags().StringP("config", "c", "env-lease.toml", "Path to config file.")
	statusCmd.Flags().String("local-config", "", "Path to local override config file.")
	rootCmd.AddCommand(statusCmd)
}
