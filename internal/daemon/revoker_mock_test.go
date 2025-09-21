package daemon

type mockRevoker struct {
	RevokeFunc func(lease Lease) error
}

func (m *mockRevoker) Revoke(lease Lease) error {
	if m.RevokeFunc != nil {
		return m.RevokeFunc(lease)
	}
	return nil
}
