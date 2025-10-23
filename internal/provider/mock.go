package provider

import (
	"fmt"

	"github.com/mblarsen/env-lease/internal/config"
)

// MockProvider is a fake secret provider for testing.
type MockProvider struct{}

// Fetch returns a dummy secret value, or an error if the sourceURI is "mock-fail".
func (p *MockProvider) Fetch(sourceURI string) (string, error) {
	if sourceURI == "mock-fail" {
		return "", fmt.Errorf("failed to fetch mock secret")
	}
	if sourceURI == "mock-explode" {
		return `{"KEY1": "VALUE1", "KEY2": "VALUE2"}`, nil
	}
	return fmt.Sprintf("secret-for-%s", sourceURI), nil
}

// FetchLeases iterates through leases and calls Fetch for each one.
func (p *MockProvider) FetchLeases(leases []config.Lease) (map[string]string, []ProviderError) {
	secrets := make(map[string]string)
	var errors []ProviderError

	for _, l := range leases {
		secret, err := p.Fetch(l.Source)
		if err != nil {
			errors = append(errors, ProviderError{Lease: l, Err: err})
			continue
		}
		secrets[l.Source] = secret
	}

	return secrets, errors
}
