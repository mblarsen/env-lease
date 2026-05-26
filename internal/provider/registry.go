package provider

import (
	"fmt"
	"os"
	"strings"
)

// DefaultProviderName is the provider used for leases that omit provider.
const DefaultProviderName = "1password"

// AdapterFactory creates a Provider adapter for one normalized account scope.
type AdapterFactory func(account string) (SecretProvider, error)

// AdapterSpec describes one Provider implementation and the URI schemes whose
// lookup behaviour belongs to that adapter.
type AdapterSpec struct {
	// Name is the canonical provider name used in lease configuration.
	Name string
	// Aliases are additional provider names accepted for this implementation.
	Aliases []string
	// BatchableSchemes are source URI schemes this adapter can fetch in a single
	// FetchLeases call. Other schemes are fetched as deduplicated singletons.
	BatchableSchemes []string
	// New constructs the account-scoped adapter.
	New AdapterFactory
}

type adapterRegistration struct {
	name             string
	batchableSchemes map[string]struct{}
	new              AdapterFactory
}

// Registry localizes Provider selection and per-provider URI capabilities.
type Registry struct {
	adapters map[string]adapterRegistration
}

// NewRegistry builds a Registry from adapter specs.
func NewRegistry(specs ...AdapterSpec) (*Registry, error) {
	r := &Registry{adapters: make(map[string]adapterRegistration)}
	for _, spec := range specs {
		if spec.Name == "" {
			return nil, fmt.Errorf("provider registry: missing adapter name")
		}
		if spec.New == nil {
			return nil, fmt.Errorf("provider registry: missing adapter factory for %q", spec.Name)
		}

		canonical := NormalizeName(spec.Name)
		registration := adapterRegistration{
			name:             canonical,
			batchableSchemes: normalizeSchemes(spec.BatchableSchemes),
			new:              spec.New,
		}
		names := append([]string{spec.Name}, spec.Aliases...)
		for i, name := range names {
			if i > 0 && strings.TrimSpace(name) == "" {
				continue
			}
			key := NormalizeName(name)
			if existing, ok := r.adapters[key]; ok {
				return nil, fmt.Errorf("provider registry: provider %q already registered as %q", key, existing.name)
			}
			r.adapters[key] = registration
		}
	}
	return r, nil
}

// DefaultRegistry returns the built-in Provider adapters.
func DefaultRegistry() *Registry {
	factory := func(account string) (SecretProvider, error) {
		return &OnePasswordCLI{Account: account}, nil
	}
	if os.Getenv("ENV_LEASE_TEST") == "1" {
		factory = func(account string) (SecretProvider, error) {
			return &MockProvider{}, nil
		}
	}

	r, err := NewRegistry(AdapterSpec{
		Name:             DefaultProviderName,
		Aliases:          []string{"op", "onepassword"},
		BatchableSchemes: []string{"op"},
		New:              factory,
	})
	if err != nil {
		panic(err)
	}
	return r
}

// NewAdapter creates the adapter registered for providerName.
func (r *Registry) NewAdapter(providerName, account string) (SecretProvider, error) {
	registration, err := r.registration(providerName)
	if err != nil {
		return nil, err
	}
	return registration.new(account)
}

// CanonicalName returns the normalized canonical provider name.
func (r *Registry) CanonicalName(providerName string) (string, error) {
	registration, err := r.registration(providerName)
	if err != nil {
		return "", err
	}
	return registration.name, nil
}

// CanBatch reports whether providerName owns batched lookup for sourceURI's scheme.
func (r *Registry) CanBatch(providerName, sourceURI string) bool {
	registration, err := r.registration(providerName)
	if err != nil {
		return false
	}
	_, ok := registration.batchableSchemes[SourceScheme(sourceURI)]
	return ok
}

// NormalizeName normalizes a lease provider name for selection.
func NormalizeName(providerName string) string {
	name := strings.ToLower(strings.TrimSpace(providerName))
	if name == "" {
		return DefaultProviderName
	}
	return name
}

// SourceScheme returns the normalized URI scheme for a lease source.
func SourceScheme(sourceURI string) string {
	before, _, ok := strings.Cut(strings.TrimSpace(sourceURI), "://")
	if !ok {
		return ""
	}
	return strings.ToLower(before)
}

func (r *Registry) registration(providerName string) (adapterRegistration, error) {
	name := NormalizeName(providerName)
	registration, ok := r.adapters[name]
	if !ok {
		return adapterRegistration{}, fmt.Errorf("unknown provider %q", providerName)
	}
	return registration, nil
}

func normalizeSchemes(schemes []string) map[string]struct{} {
	out := make(map[string]struct{}, len(schemes))
	for _, scheme := range schemes {
		scheme = strings.ToLower(strings.TrimSpace(strings.TrimSuffix(scheme, "://")))
		if scheme != "" {
			out[scheme] = struct{}{}
		}
	}
	return out
}
