package daemon

import "github.com/mblarsen/env-lease/internal/config"

type mockRevoker struct {
	RevokeCount int
	RevokeFunc  func(lease *config.Lease) error
}

func (m *mockRevoker) Revoke(lease *config.Lease) error {
	m.RevokeCount++
	if m.RevokeFunc != nil {
		return m.RevokeFunc(lease)
	}
	return nil
}
