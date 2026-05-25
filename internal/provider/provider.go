package provider

import "github.com/mblarsen/env-lease/internal/lease"

// ProviderError associates an error with a specific lease that failed.
type ProviderError struct {
	Lease lease.Lease
	Err   error
}

// SecretProvider defines the low-level adapter interface for fetching Secrets
// from a Provider. Callers choose the concrete Provider/account adapter before
// crossing this seam.
type SecretProvider interface {
	// Fetch retrieves a Secret from the given source URI.
	Fetch(sourceURI string) (string, error)
	// FetchLeases retrieves Secrets for a slice of Leases using this adapter's
	// configured account. RETURN CONTRACT: the returned map MUST be keyed by
	// Lease.Source (the source URI).
	FetchLeases(leases []lease.Lease) (map[string]string, []ProviderError)
}

// BulkSecretProvider defines the interface for providers that can fetch multiple
// secrets in a single operation.
type BulkSecretProvider interface {
	SecretProvider
	// FetchBulk retrieves multiple secrets from the given source URIs.
	FetchBulk(sources map[string]string) (map[string]string, error)
}
