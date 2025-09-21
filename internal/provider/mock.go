package provider

import "fmt"

// MockProvider is a fake secret provider for testing.
type MockProvider struct{}

// Fetch returns a dummy secret value.
func (p *MockProvider) Fetch(sourceURI string) (string, error) {
	return fmt.Sprintf("secret-for-%s", sourceURI), nil
}
