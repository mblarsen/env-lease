package provider

import (
	"strings"
	"testing"
)

func TestDefaultRegistrySelectsOnePasswordAdapter(t *testing.T) {
	t.Setenv("ENV_LEASE_TEST", "")

	adapter, err := DefaultRegistry().NewAdapter("1password", "team-account")
	if err != nil {
		t.Fatalf("NewAdapter returned error: %v", err)
	}

	op, ok := adapter.(*OnePasswordCLI)
	if !ok {
		t.Fatalf("adapter type = %T, want *OnePasswordCLI", adapter)
	}
	if op.Account != "team-account" {
		t.Fatalf("account = %q, want %q", op.Account, "team-account")
	}
}

func TestDefaultRegistryNormalizesAliasesAndDefaultProvider(t *testing.T) {
	t.Setenv("ENV_LEASE_TEST", "")
	registry := DefaultRegistry()

	for _, name := range []string{"", "1PASSWORD", " onepassword ", "op"} {
		canonical, err := registry.CanonicalName(name)
		if err != nil {
			t.Fatalf("CanonicalName(%q) returned error: %v", name, err)
		}
		if canonical != "1password" {
			t.Fatalf("CanonicalName(%q) = %q, want 1password", name, canonical)
		}
	}
}

func TestDefaultRegistryReportsUnknownProvider(t *testing.T) {
	_, err := DefaultRegistry().NewAdapter("vault", "")
	if err == nil {
		t.Fatal("expected unknown provider error")
	}
	if !strings.Contains(err.Error(), `unknown provider "vault"`) {
		t.Fatalf("error = %q, want unknown provider", err.Error())
	}
}

func TestDefaultRegistryBatchPolicyIsSchemeSpecific(t *testing.T) {
	registry := DefaultRegistry()

	if !registry.CanBatch("1password", "op://vault/item/field") {
		t.Fatal("expected op:// to be batchable for 1password")
	}
	if registry.CanBatch("1password", "op+file://item/file.json") {
		t.Fatal("expected op+file:// to be fetched as singleton for 1password")
	}
	if registry.CanBatch("vault", "op://vault/item/field") {
		t.Fatal("expected unknown providers to be non-batchable")
	}
}

func TestNewRegistrySupportsAdditionalProviderWithoutLookupChanges(t *testing.T) {
	registry, err := NewRegistry(AdapterSpec{
		Name:             "vault",
		BatchableSchemes: []string{"vault"},
		New: func(account string) (SecretProvider, error) {
			return &MockProvider{}, nil
		},
	})
	if err != nil {
		t.Fatalf("NewRegistry returned error: %v", err)
	}

	if !registry.CanBatch("vault", "vault://secret/data/app#token") {
		t.Fatal("expected registered vault:// scheme to be batchable")
	}
	adapter, err := registry.NewAdapter("vault", "")
	if err != nil {
		t.Fatalf("NewAdapter returned error: %v", err)
	}
	if _, ok := adapter.(*MockProvider); !ok {
		t.Fatalf("adapter type = %T, want *MockProvider", adapter)
	}
}
