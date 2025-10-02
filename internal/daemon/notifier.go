package daemon

import (
	"os"
	"runtime"

	"github.com/gen2brain/beeep"
	"github.com/mblarsen/env-lease/assets"
)

// Notifier is an interface for sending desktop notifications.
type Notifier interface {
	// Notify sends a desktop notification.
	Notify(title, message string) error
}

// BeeepNotifier sends notifications using the beeep library.
type BeeepNotifier struct{}

// Notify sends a desktop notification.
func (n *BeeepNotifier) Notify(title, message string) error {
	// On macOS, Alert() is needed to show a custom icon, as it creates a temporary app bundle.
	if runtime.GOOS == "darwin" {
		tmpfile, err := os.CreateTemp("", "env-lease-icon-*.png")
		if err != nil {
			return err
		}
		defer os.Remove(tmpfile.Name())

		if _, err := tmpfile.Write(assets.IconData); err != nil {
			return err
		}
		if err := tmpfile.Close(); err != nil {
			return err
		}

		return beeep.Alert(title, message, tmpfile.Name())
	}

	// For other systems like Linux, Notify() is sufficient.
	return beeep.Notify(title, message, "") // Icon path is often ignored on Linux anyway
}
