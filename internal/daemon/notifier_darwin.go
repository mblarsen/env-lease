//go:build darwin

package daemon

import (
	"fmt"
	"os/exec"
)

// OsaScriptNotifier sends notifications on macOS using osascript.
type OsaScriptNotifier struct{}

// Notify sends a desktop notification.
func (n *OsaScriptNotifier) Notify(title, message string) error {
	cmd := exec.Command("osascript", "-e", fmt.Sprintf("display notification \"%s\" with title \"%s\"", message, title))
	return cmd.Run()
}
