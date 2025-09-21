package provider

// SecretProvider defines the interface for fetching secrets from a backend.
type SecretProvider interface {
	// Fetch retrieves a secret from the given source URI.
	Fetch(sourceURI string) (string, error)
}
