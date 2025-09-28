package daemon

// Notifier is an interface for sending desktop notifications.
type Notifier interface {
	// Notify sends a desktop notification.
	Notify(title, message string) error
}
