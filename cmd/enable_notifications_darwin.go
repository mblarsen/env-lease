//go:build darwin

package cmd

import (
	"fmt"
	"os/exec"

	"github.com/spf13/cobra"
)

const appleScriptContent = `display notification "Notifications are now enabled for env-lease!" with title "env-lease"`

var enableNotificationsCmd = &cobra.Command{
	Use:   "enable-notifications",
	Short: "Guides you to enable notifications on macOS.",
	Long: `Opens Script Editor with a pre-filled script to enable notifications.

You must run this script once and grant permissions for env-lease to be able to
send you notifications when your leases expire.`,
	RunE: func(cmd *cobra.Command, args []string) error {
		fmt.Println("Opening Script Editor...")
		fmt.Println("Please click the 'Run' button (the triangle) in Script Editor.")
		fmt.Println("Then, click 'Allow' when prompted for notification permissions.")

		// Use osascript to open Script Editor with the content, as it's more direct
		// than creating a temporary file.
		command := fmt.Sprintf("tell application \"Script Editor\" to make new document with properties {contents:\"%s\"}\nactivate", appleScriptContent)
		runCmd := exec.Command("osascript", "-e", command)
		return runCmd.Run()
	},
}

func init() {
	rootCmd.AddCommand(enableNotificationsCmd)
}
