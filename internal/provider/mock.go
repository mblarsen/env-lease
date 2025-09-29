package provider

import "fmt"

// MockProvider is a fake secret provider for testing.
type MockProvider struct{}

// Fetch returns a dummy secret value, or an error if the sourceURI is "mock-fail".
func (p *MockProvider) Fetch(sourceURI string) (string, error) {
	if sourceURI == "mock-fail" {
		return "", fmt.Errorf("failed to fetch mock secret")
	}
	return fmt.Sprintf("secret-for-%s", sourceURI), nil
}
