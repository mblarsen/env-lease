package daemon

import (
	"github.com/mblarsen/env-lease/internal/destination"
	"github.com/mblarsen/env-lease/internal/lease"
)

// Revoker is an interface for revoking lease destination effects.
type Revoker interface {
	Revoke(lease *lease.Lease) (destination.Revoked, error)
}

// FileRevoker is a revoker that modifies filesystem and shell destinations.
type FileRevoker struct{}

// Revoke revokes a lease by delegating destination mutation to the destination module.
func (r *FileRevoker) Revoke(l *lease.Lease) (destination.Revoked, error) {
	return (destination.Revoker{}).Revoke(l)
}
