//go:build darwin

package cmd

import (
	"fmt"
	"os"
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

		// Create a temporary file to hold the AppleScript.
		// This is more robust than passing the script as a command-line argument.
		tmpfile, err := os.CreateTemp("", "env-lease-*.applescript")
		if err != nil {
			return fmt.Errorf("failed to create temporary script file: %w", err)
		}

		if _, err := tmpfile.WriteString(appleScriptContent); err != nil {
			tmpfile.Close() // best effort
			return fmt.Errorf("failed to write to temporary script file: %w", err)
		}
		if err := tmpfile.Close(); err != nil {
			return fmt.Errorf("failed to close temporary script file: %w", err)
		}

		// Use 'open' to launch the script file in Script Editor.
		runCmd := exec.Command("open", "-a", "Script Editor", tmpfile.Name())
		if err := runCmd.Run(); err != nil {
			return fmt.Errorf("failed to open Script Editor: %w", err)
		}
		return nil
	},
}

func init() {
	rootCmd.AddCommand(enableNotificationsCmd)
}
