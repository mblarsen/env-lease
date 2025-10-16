package provider

// SecretProvider defines the interface for fetching secrets from a backend.
type SecretProvider interface {
	// Fetch retrieves a secret from the given source URI.
	Fetch(sourceURI string) (string, error)
}

// BulkSecretProvider defines the interface for providers that can fetch multiple
// secrets in a single operation.
type BulkSecretProvider interface {
	SecretProvider
	// FetchBulk retrieves multiple secrets from the given source URIs.
	FetchBulk(sources map[string]string) (map[string]string, error)
}
