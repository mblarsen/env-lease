//go:build darwin
// +build darwin

package cmd

import (
	"fmt"
	"github.com/mblarsen/env-lease/internal/fileutil"
	"github.com/spf13/cobra"
	"os"
	"os/exec"
	"path/filepath"
)

var daemonCmd = &cobra.Command{
	Use:   "daemon",
	Short: "Manage the env-lease daemon.",
	Long:  `Manage the env-lease daemon.`,
}

var installCmd = &cobra.Command{
	Use:   "install",
	Short: "Install the env-lease daemon.",
	Long:  `Install the env-lease daemon.`,
	RunE: func(cmd *cobra.Command, args []string) error {
		executable, err := os.Executable()
		if err != nil {
			return err
		}

		plist := fmt.Sprintf(plistTemplate, executable)
		plistPath := filepath.Join(os.Getenv("HOME"), "Library", "LaunchAgents", "com.user.env-leased.plist")

		if err := fileutil.AtomicWriteFile(plistPath, []byte(plist), 0644); err != nil {
			return err
		}

		return exec.Command("launchctl", "load", plistPath).Run()
	},
}

var uninstallCmd = &cobra.Command{
	Use:   "uninstall",
	Short: "Uninstall the env-lease daemon.",
	Long:  `Uninstall the env-lease daemon.`,
	RunE: func(cmd *cobra.Command, args []string) error {
		plistPath := filepath.Join(os.Getenv("HOME"), "Library", "LaunchAgents", "com.user.env-leased.plist")

		if err := exec.Command("launchctl", "unload", plistPath).Run(); err != nil {
			// Ignore errors, as the service may not be loaded
		}

		return os.Remove(plistPath)
	},
}

const plistTemplate = `<?xml version="1.0" encoding="UTF-8"?>
<!DOCTYPE plist PUBLIC "-//Apple//DTD PLIST 1.0//EN" "http://www.apple.com/DTDs/PropertyList-1.0.dtd">
<plist version="1.0">
<dict>
	<key>Label</key>
	<string>com.user.env-leased</string>
	<key>ProgramArguments</key>
	<array>
		<string>%s</string>
		<string>daemon</string>
		<string>run</string>
	</array>
	<key>RunAtLoad</key>
	<true/>
	<key>KeepAlive</key>
	<true/>
</dict>
</plist>
`

func init() {
	daemonCmd.AddCommand(installCmd)
	daemonCmd.AddCommand(uninstallCmd)
	rootCmd.AddCommand(daemonCmd)
}
