package cmd

import (
	"fmt"
	"os"
	"sort"
	"text/tabwriter"
	"time"

	"github.com/mblarsen/env-lease/internal/config"
	"github.com/mblarsen/env-lease/internal/ipc"
	"github.com/spf13/cobra"
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

		configFileFlag, _ := cmd.Flags().GetString("config")
		absConfigFile, err := config.ResolveConfigFile(configFileFlag)
		if err != nil {
			return err
		}

		// The status command doesn't need to load the config, it just needs the path
		// to filter leases. So we don't call config.Load here. But if we did, it
		// would look like this:
		// localConfigFileFlag, _ := cmd.Flags().GetString("local-config")
		// _, err = config.Load(absConfigFile, localConfigFileFlag)
		// if err != nil {
		// 	return err
		// }

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
			fmt.Println("No active leases for this project.")
		} else {
			// Sort top-level leases by destination
			sort.Slice(leasesToDisplay, func(i, j int) bool {
				return leasesToDisplay[i].Destination < leasesToDisplay[j].Destination
			})
			printLeases(leasesToDisplay, groupedLeases)
		}

		// Calculate other leases count, excluding parent leases from the count
		if !showAll {
			var allLeasesCount int
			for _, lease := range allTopLevelLeases {
				uniqueParentID := lease.Source + "->" + lease.Destination
				if children, isParent := groupedLeases[uniqueParentID]; isParent {
					allLeasesCount += len(children)
				} else {
					allLeasesCount++
				}
			}

			var displayedLeasesCount int
			for _, lease := range leasesToDisplay {
				uniqueParentID := lease.Source + "->" + lease.Destination
				if children, isParent := groupedLeases[uniqueParentID]; isParent {
					displayedLeasesCount += len(children)
				} else {
					displayedLeasesCount++
				}
			}

			otherLeasesCount := allLeasesCount - displayedLeasesCount
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
			if lease.LeaseType == "file" {
				variable = "<file>"
			} else {
				variable = "<exploded>"
			}
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
	statusCmd.Flags().StringP("config", "c", "env-lease.toml", "Path to config file.")
	statusCmd.Flags().String("local-config", "", "Path to local override config file.")
	rootCmd.AddCommand(statusCmd)
}
