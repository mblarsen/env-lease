package daemon

// Revoker is an interface for revoking leases.
type Revoker interface {
	Revoke(lease Lease) error
}
