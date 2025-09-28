package daemon

type mockNotifier struct {
	NotifyCount int
	LastTitle   string
	LastMessage string
}

func (m *mockNotifier) Notify(title, message string) error {
	m.NotifyCount++
	m.LastTitle = title
	m.LastMessage = message
	return nil
}
