package daemon

import "github.com/mblarsen/env-lease/internal/lease"

type mockRevoker struct {
	RevokeCount int
	RevokeFunc  func(lease *lease.Lease) error
	revoked     []*lease.Lease
}

func (m *mockRevoker) Revoke(lease *lease.Lease) error {
	m.RevokeCount++
	m.revoked = append(m.revoked, lease)
	if m.RevokeFunc != nil {
		return m.RevokeFunc(lease)
	}
	return nil
}
