package provider

import "github.com/mblarsen/env-lease/internal/config"

// ProviderError associates an error with a specific lease that failed.
type ProviderError struct {
	Lease config.Lease
	Err   error
}

// SecretProvider defines the interface for fetching secrets from a backend.
type SecretProvider interface {
	// Fetch retrieves a secret from the given source URI.
	Fetch(sourceURI string) (string, error)
	// FetchLeases retrieves secrets for a slice of leases.
	// RETURN CONTRACT: the returned map MUST be keyed by Lease.Source (the source URI).
	// This ensures a stable key across simple, file, and explode flows.
	FetchLeases(leases []config.Lease) (map[string]string, []ProviderError)
}

// BulkSecretProvider defines the interface for providers that can fetch multiple
// secrets in a single operation.
type BulkSecretProvider interface {
	SecretProvider
	// FetchBulk retrieves multiple secrets from the given source URIs.
	FetchBulk(sources map[string]string) (map[string]string, error)
}
