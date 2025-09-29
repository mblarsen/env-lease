package daemon

import "github.com/mblarsen/env-lease/internal/config"

type mockRevoker struct {
	RevokeCount int
	RevokeFunc  func(lease *config.Lease) error
	revoked     []*config.Lease
}

func (m *mockRevoker) Revoke(lease *config.Lease) error {
	m.RevokeCount++
	m.revoked = append(m.revoked, lease)
	if m.RevokeFunc != nil {
		return m.RevokeFunc(lease)
	}
	return nil
}
