package secretlookup

import (
	"fmt"
	"strings"
	"sync"
	"testing"

	"github.com/mblarsen/env-lease/internal/lease"
	"github.com/mblarsen/env-lease/internal/provider"
)

type recordingProvider struct {
	providerName string
	account      string

	mu                sync.Mutex
	fetchLeasesCalls  [][]lease.Lease
	fetchCalls        []string
	fetchFailures     map[string]error
	fetchLeaseFailure error
}

func (p *recordingProvider) Fetch(sourceURI string) (string, error) {
	p.mu.Lock()
	defer p.mu.Unlock()

	p.fetchCalls = append(p.fetchCalls, sourceURI)
	if err := p.fetchFailures[sourceURI]; err != nil {
		return "", err
	}
	return fmt.Sprintf("secret:%s:%s:%s", p.providerName, p.account, sourceURI), nil
}

func (p *recordingProvider) FetchLeases(leases []lease.Lease) (map[string]string, []provider.ProviderError) {
	p.mu.Lock()
	defer p.mu.Unlock()

	p.fetchLeasesCalls = append(p.fetchLeasesCalls, append([]lease.Lease(nil), leases...))
	if p.fetchLeaseFailure != nil {
		errs := make([]provider.ProviderError, 0, len(leases))
		for _, l := range leases {
			errs = append(errs, provider.ProviderError{Lease: l, Err: p.fetchLeaseFailure})
		}
		return nil, errs
	}

	secrets := make(map[string]string, len(leases))
	for _, l := range leases {
		secrets[l.Source] = fmt.Sprintf("secret:%s:%s:%s", p.providerName, p.account, l.Source)
	}
	return secrets, nil
}

type recordingFactory struct {
	mu        sync.Mutex
	providers map[string]*recordingProvider
}

func newRecordingFactory() *recordingFactory {
	return &recordingFactory{providers: make(map[string]*recordingProvider)}
}

func (f *recordingFactory) factory(providerName, account string) (provider.SecretProvider, error) {
	f.mu.Lock()
	defer f.mu.Unlock()

	key := providerName + "/" + account
	p := f.providers[key]
	if p == nil {
		p = &recordingProvider{
			providerName:  providerName,
			account:       account,
			fetchFailures: map[string]error{},
		}
		f.providers[key] = p
	}
	return p, nil
}

func (f *recordingFactory) provider(providerName, account string) *recordingProvider {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.providers[providerName+"/"+account]
}

func TestLookupFetch_GroupsByProviderAccountAndDedupeSource(t *testing.T) {
	factory := newRecordingFactory()
	lookup := NewWithProviderFactory(factory.factory)

	accountAShared := lease.Lease{Provider: "1password", Source: "op://vault/shared", OpAccount: "account-a", Variable: "A_SHARED"}
	accountAOther := lease.Lease{Provider: "1password", Source: "op://vault/other", OpAccount: "account-a", Variable: "A_OTHER"}
	accountBShared := lease.Lease{Provider: "1password", Source: "op://vault/shared", OpAccount: "account-b", Variable: "B_SHARED"}
	fileOne := lease.Lease{Provider: "1password", Source: "op+file://item/file.json", OpAccount: "account-a", Variable: "FILE_ONE"}
	fileTwo := lease.Lease{Provider: "1password", Source: "op+file://item/file.json", OpAccount: "account-a", Variable: "FILE_TWO"}

	secrets, errs, err := lookup.Fetch([]lease.Lease{accountAShared, accountAOther, accountBShared, fileOne, fileTwo}, Options{})
	if err != nil {
		t.Fatalf("unexpected fetch error: %v", err)
	}
	if len(errs) != 0 {
		t.Fatalf("unexpected per-lease errors: %v", errs)
	}

	assertSecret := func(l lease.Lease, want string) {
		t.Helper()
		got, ok := secrets.Get(l)
		if !ok {
			t.Fatalf("missing secret for %#v", l)
		}
		if got != want {
			t.Fatalf("secret mismatch for %s/%s: got %q want %q", l.OpAccount, l.Source, got, want)
		}
	}

	assertSecret(accountAShared, "secret:1password:account-a:op://vault/shared")
	assertSecret(accountAOther, "secret:1password:account-a:op://vault/other")
	assertSecret(accountBShared, "secret:1password:account-b:op://vault/shared")
	assertSecret(fileOne, "secret:1password:account-a:op+file://item/file.json")
	assertSecret(fileTwo, "secret:1password:account-a:op+file://item/file.json")

	accountAProvider := factory.provider("1password", "account-a")
	if accountAProvider == nil {
		t.Fatal("missing account-a provider")
	}
	if got := len(accountAProvider.fetchLeasesCalls); got != 1 {
		t.Fatalf("account-a bulk calls = %d, want 1", got)
	}
	if got := len(accountAProvider.fetchLeasesCalls[0]); got != 2 {
		t.Fatalf("account-a bulk lease count = %d, want 2", got)
	}
	if got := len(accountAProvider.fetchCalls); got != 1 {
		t.Fatalf("account-a singleton fetch calls = %d, want 1", got)
	}

	accountBProvider := factory.provider("1password", "account-b")
	if accountBProvider == nil {
		t.Fatal("missing account-b provider")
	}
	if got := len(accountBProvider.fetchLeasesCalls); got != 1 {
		t.Fatalf("account-b bulk calls = %d, want 1", got)
	}
	if got := len(accountBProvider.fetchCalls); got != 0 {
		t.Fatalf("account-b singleton fetch calls = %d, want 0", got)
	}
}

func TestLookupFetch_CanonicalizesProviderAliasesBeforeGrouping(t *testing.T) {
	factory := newRecordingFactory()
	lookup := NewWithProviderFactory(factory.factory)

	opAlias := lease.Lease{Provider: "op", Source: "op://vault/shared", OpAccount: "account-a", Variable: "OP_ALIAS"}
	nameAlias := lease.Lease{Provider: "onepassword", Source: "op://vault/shared", OpAccount: "account-a", Variable: "NAME_ALIAS"}
	caseAlias := lease.Lease{Provider: "1PASSWORD", Source: "op://vault/other", OpAccount: "account-a", Variable: "CASE_ALIAS"}

	_, errs, err := lookup.Fetch([]lease.Lease{opAlias, nameAlias, caseAlias}, Options{})
	if err != nil {
		t.Fatalf("unexpected fetch error: %v", err)
	}
	if len(errs) != 0 {
		t.Fatalf("unexpected per-lease errors: %v", errs)
	}

	canonicalProvider := factory.provider("1password", "account-a")
	if canonicalProvider == nil {
		t.Fatal("missing canonical 1password provider")
	}
	if got := len(canonicalProvider.fetchLeasesCalls); got != 1 {
		t.Fatalf("bulk calls = %d, want 1", got)
	}
	if got := len(canonicalProvider.fetchLeasesCalls[0]); got != 3 {
		t.Fatalf("bulk lease count = %d, want 3", got)
	}
	if factory.provider("op", "account-a") != nil || factory.provider("onepassword", "account-a") != nil || factory.provider("1PASSWORD", "account-a") != nil {
		t.Fatal("expected aliases to share only the canonical provider adapter")
	}
}

func TestLookupFetch_CustomProviderFactoryKeepsLegacyOpBatching(t *testing.T) {
	factory := newRecordingFactory()
	lookup := NewWithProviderFactory(factory.factory)

	first := lease.Lease{Provider: "vault", Source: "op://vault/first", Variable: "FIRST"}
	second := lease.Lease{Provider: "vault", Source: "op://vault/second", Variable: "SECOND"}
	file := lease.Lease{Provider: "vault", Source: "vault://secret/data/app#local", Variable: "LOCAL"}

	_, errs, err := lookup.Fetch([]lease.Lease{first, second, file}, Options{})
	if err != nil {
		t.Fatalf("unexpected fetch error: %v", err)
	}
	if len(errs) != 0 {
		t.Fatalf("unexpected per-lease errors: %v", errs)
	}

	vaultProvider := factory.provider("vault", "")
	if vaultProvider == nil {
		t.Fatal("missing vault provider")
	}
	if got := len(vaultProvider.fetchLeasesCalls); got != 1 {
		t.Fatalf("bulk calls = %d, want 1", got)
	}
	if got := len(vaultProvider.fetchLeasesCalls[0]); got != 2 {
		t.Fatalf("bulk lease count = %d, want 2", got)
	}
	if got := len(vaultProvider.fetchCalls); got != 1 {
		t.Fatalf("singleton fetch calls = %d, want 1", got)
	}
}

func TestLookupFetch_UsesProviderBatchPolicyForNonOnePasswordSchemes(t *testing.T) {
	factory := newRecordingFactory()
	lookup := NewWithProviderFactoryAndBatchPolicy(factory.factory, func(providerName, sourceURI string) bool {
		return providerName == "vault" && strings.HasPrefix(sourceURI, "vault://")
	})

	first := lease.Lease{Provider: "vault", Source: "vault://secret/data/app#api_key", Variable: "API_KEY"}
	second := lease.Lease{Provider: "vault", Source: "vault://secret/data/app#db_url", Variable: "DB_URL"}
	file := lease.Lease{Provider: "vault", Source: "file://local.json", Variable: "LOCAL"}

	secrets, errs, err := lookup.Fetch([]lease.Lease{first, second, file}, Options{})
	if err != nil {
		t.Fatalf("unexpected fetch error: %v", err)
	}
	if len(errs) != 0 {
		t.Fatalf("unexpected per-lease errors: %v", errs)
	}

	vaultProvider := factory.provider("vault", "")
	if vaultProvider == nil {
		t.Fatal("missing vault provider")
	}
	if got := len(vaultProvider.fetchLeasesCalls); got != 1 {
		t.Fatalf("bulk calls = %d, want 1", got)
	}
	if got := len(vaultProvider.fetchLeasesCalls[0]); got != 2 {
		t.Fatalf("bulk lease count = %d, want 2", got)
	}
	if got := len(vaultProvider.fetchCalls); got != 1 {
		t.Fatalf("singleton fetch calls = %d, want 1", got)
	}
	if got, ok := secrets.Get(file); !ok || got != "secret:vault::file://local.json" {
		t.Fatalf("singleton secret = %q, %v", got, ok)
	}
}

func TestLookupFetch_UnknownProviderReturnsLeaseError(t *testing.T) {
	missing := lease.Lease{Provider: "vault", Source: "vault://secret/data/app#api_key", Variable: "API_KEY"}

	_, errs, err := New().Fetch([]lease.Lease{missing}, Options{})
	if err == nil {
		t.Fatal("expected top-level error")
	}
	if len(errs) != 1 {
		t.Fatalf("per-lease errors = %d, want 1", len(errs))
	}
	if errs[0].Lease.Provider != missing.Provider || errs[0].Lease.Source != missing.Source || errs[0].Lease.Variable != missing.Variable {
		t.Fatalf("error lease = %#v, want %#v", errs[0].Lease, missing)
	}
	if !strings.Contains(errs[0].Err.Error(), `unknown provider "vault"`) {
		t.Fatalf("per-lease error = %q, want unknown provider", errs[0].Err.Error())
	}
}

func TestLookupFetch_ContinueOnError(t *testing.T) {
	factory := newRecordingFactory()
	failingSource := "op+file://item/missing.json"

	failingProvider := &recordingProvider{
		providerName:  "1password",
		account:       "account-a",
		fetchFailures: map[string]error{failingSource: fmt.Errorf("missing file")},
	}
	factory.providers["1password/account-a"] = failingProvider

	lookup := NewWithProviderFactory(factory.factory)
	success := lease.Lease{Provider: "1password", Source: "op://vault/ok", OpAccount: "account-a", Variable: "OK"}
	failure := lease.Lease{Provider: "1password", Source: failingSource, OpAccount: "account-a", Variable: "FAIL"}

	secrets, errs, err := lookup.Fetch([]lease.Lease{success, failure}, Options{ContinueOnError: true})
	if err != nil {
		t.Fatalf("expected no top-level error with ContinueOnError, got %v", err)
	}
	if len(errs) != 1 {
		t.Fatalf("per-lease errors = %d, want 1", len(errs))
	}
	if errs[0].Lease.Source != failingSource {
		t.Fatalf("error source = %q, want %q", errs[0].Lease.Source, failingSource)
	}
	if got, ok := secrets.Get(success); !ok || got != "secret:1password:account-a:op://vault/ok" {
		t.Fatalf("successful secret = %q, %v", got, ok)
	}
}

func TestLookupFetch_StopsOnError(t *testing.T) {
	factory := newRecordingFactory()
	failingSource := "op+file://item/missing.json"
	failingProvider := &recordingProvider{
		providerName:  "1password",
		account:       "account-a",
		fetchFailures: map[string]error{failingSource: fmt.Errorf("missing file")},
	}
	factory.providers["1password/account-a"] = failingProvider

	lookup := NewWithProviderFactory(factory.factory)
	failure := lease.Lease{Provider: "1password", Source: failingSource, OpAccount: "account-a", Variable: "FAIL"}

	_, errs, err := lookup.Fetch([]lease.Lease{failure}, Options{})
	if err == nil {
		t.Fatal("expected top-level error")
	}
	if len(errs) != 1 {
		t.Fatalf("per-lease errors = %d, want 1", len(errs))
	}
}
