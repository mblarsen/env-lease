//go:build linux

package daemon

import (
	"os/exec"
)

// NotifySendNotifier sends notifications on Linux using notify-send.
type NotifySendNotifier struct{}

// Notify sends a desktop notification.
func (n *NotifySendNotifier) Notify(title, message string) error {
	cmd := exec.Command("notify-send", title, message)
	return cmd.Run()
}
