package daemon

import (
	"github.com/mblarsen/env-lease/internal/destination"
	"github.com/mblarsen/env-lease/internal/lease"
)

type mockRevoker struct {
	RevokeCount int
	RevokeFunc  func(lease *lease.Lease) (destination.Revoked, error)
	revoked     []*lease.Lease
}

func (m *mockRevoker) Revoke(l *lease.Lease) (destination.Revoked, error) {
	m.RevokeCount++
	m.revoked = append(m.revoked, l)
	if m.RevokeFunc != nil {
		return m.RevokeFunc(l)
	}
	if l != nil && l.LeaseType == lease.TypeShell && l.Variable != "" {
		return destination.Revoked{ShellCommands: []string{"unset " + l.Variable}}, nil
	}
	return destination.Revoked{}, nil
}
