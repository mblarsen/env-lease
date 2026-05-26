// Package secretlookup owns Secret lookup orchestration for Grant flows.
package secretlookup

import (
	"fmt"
	"log/slog"
	"strings"
	"sync"

	"github.com/mblarsen/env-lease/internal/lease"
	"github.com/mblarsen/env-lease/internal/provider"
	"golang.org/x/sync/errgroup"
)

// Error associates a Secret lookup failure with the Lease that needed it.
type Error struct {
	Lease lease.Lease
	Err   error
}

// Options controls Secret lookup behaviour.
type Options struct {
	// ContinueOnError keeps resolving independent Leases after a lookup failure.
	ContinueOnError bool
	// Mode is included in diagnostic logs to identify the caller flow.
	Mode string
}

// Secrets contains fetched Secret material keyed by the lookup facts for a Lease.
type Secrets struct {
	byKey map[key]string
}

// Get returns the fetched Secret material for l.
func (s Secrets) Get(l lease.Lease) (string, bool) {
	if s.byKey == nil {
		return "", false
	}
	secret, ok := s.byKey[keyFor(l)]
	return secret, ok
}

func (s *Secrets) set(l lease.Lease, secret string) {
	if s.byKey == nil {
		s.byKey = make(map[key]string)
	}
	s.byKey[keyFor(l)] = secret
}

// ProviderFactory creates Provider adapters for a normalized provider/account pair.
type ProviderFactory func(providerName, account string) (provider.SecretProvider, error)

// BatchPolicy reports whether providerName can batch sourceURI through FetchLeases.
type BatchPolicy func(providerName, sourceURI string) bool

// ProviderNormalizer returns the canonical name used for Provider grouping and selection.
type ProviderNormalizer func(providerName string) string

// Lookup resolves Secret material for normalized Leases.
type Lookup struct {
	providerFactory   ProviderFactory
	canBatch          BatchPolicy
	normalizeProvider ProviderNormalizer
}

// New creates a Secret lookup Module with the default Provider adapter factory.
func New() *Lookup {
	return NewWithProviderFactoryAndBatchPolicy(defaultProviderFactory, defaultBatchPolicy)
}

// NewWithProviderFactory creates a Secret lookup Module with a custom Provider
// adapter factory. It preserves the legacy custom-factory batching behaviour:
// op:// sources are sent through FetchLeases unless the caller opts into an
// explicit policy with NewWithProviderFactoryAndBatchPolicy.
func NewWithProviderFactory(factory ProviderFactory) *Lookup {
	return NewWithProviderFactoryAndBatchPolicy(factory, legacyBatchPolicy)
}

// NewWithProviderFactoryAndBatchPolicy creates a Secret lookup Module with a
// custom Provider adapter factory and explicit provider URI batching policy.
func NewWithProviderFactoryAndBatchPolicy(factory ProviderFactory, canBatch BatchPolicy) *Lookup {
	if factory == nil {
		factory = defaultProviderFactory
	}
	if canBatch == nil {
		canBatch = defaultBatchPolicy
	}
	return &Lookup{providerFactory: factory, canBatch: canBatch, normalizeProvider: defaultProviderNormalizer}
}

// Fetch resolves Secret material for leases using the default lookup Module.
func Fetch(leases []lease.Lease, opts Options) (Secrets, []Error, error) {
	return New().Fetch(leases, opts)
}

// Fetch resolves Secret material for leases. Sources with Provider-registered
// batchable schemes are batched by Provider and account. Other sources are
// deduplicated by Provider, account, and source URI, then fetched once and
// shared by matching Leases.
func (lookup *Lookup) Fetch(leases []lease.Lease, opts Options) (Secrets, []Error, error) {
	plan := buildPlan(leases, lookup.canBatch, lookup.normalizeProvider)

	slog.Debug("secret lookup: start",
		"mode", opts.Mode,
		"lease_count", len(leases),
		"batches", len(plan.batches),
		"single_sources", len(plan.singletons))

	secrets := Secrets{byKey: make(map[key]string, len(leases))}
	var errs []Error

	var mu sync.Mutex
	var group errgroup.Group

	for _, batch := range plan.batches {
		batch := batch
		group.Go(func() error {
			providerAdapter, err := lookup.providerFactory(batch.providerName, batch.account)
			if err != nil {
				lookupErrs := errorsForLeases(batch.leases, err)
				mu.Lock()
				errs = append(errs, lookupErrs...)
				mu.Unlock()
				if !opts.ContinueOnError {
					return fmt.Errorf("secret lookup: provider %s account %s: %w", batch.providerName, batch.account, err)
				}
				return nil
			}

			fetched, providerErrs := providerAdapter.FetchLeases(batch.leases)
			lookupErrs := lookupErrors(providerErrs)

			if len(providerErrs) == 0 {
				for _, leaseToFill := range batch.leases {
					if _, ok := fetched[leaseToFill.Source]; !ok {
						lookupErrs = append(lookupErrs, Error{
							Lease: leaseToFill,
							Err:   fmt.Errorf("no secret returned for %s", leaseToFill.Source),
						})
					}
				}
			}

			mu.Lock()
			for _, leaseToFill := range batch.leases {
				if secret, ok := fetched[leaseToFill.Source]; ok {
					secrets.set(leaseToFill, secret)
				}
			}
			if len(lookupErrs) > 0 {
				errs = append(errs, lookupErrs...)
			}
			mu.Unlock()

			slog.Debug("secret lookup: fetched batch",
				"mode", opts.Mode,
				"provider", batch.providerName,
				"account", batch.account,
				"count", len(batch.leases),
				"success_count", len(fetched),
				"error_count", len(providerErrs))

			if len(lookupErrs) > 0 && !opts.ContinueOnError {
				return fmt.Errorf("secret lookup: failed batch provider %s account %s", batch.providerName, batch.account)
			}
			return nil
		})
	}

	for _, sourceGroup := range plan.singletons {
		sourceGroup := sourceGroup
		group.Go(func() error {
			providerAdapter, err := lookup.providerFactory(sourceGroup.providerName, sourceGroup.account)
			if err != nil {
				lookupErrs := errorsForLeases(sourceGroup.leases, err)
				mu.Lock()
				errs = append(errs, lookupErrs...)
				mu.Unlock()
				if !opts.ContinueOnError {
					return fmt.Errorf("secret lookup: provider %s account %s: %w", sourceGroup.providerName, sourceGroup.account, err)
				}
				return nil
			}

			secret, err := providerAdapter.Fetch(sourceGroup.source)
			if err != nil {
				lookupErrs := errorsForLeases(sourceGroup.leases, err)
				mu.Lock()
				errs = append(errs, lookupErrs...)
				mu.Unlock()
				if !opts.ContinueOnError {
					return fmt.Errorf("secret lookup: failed source %s", sourceGroup.source)
				}
				return nil
			}

			mu.Lock()
			for _, leaseToFill := range sourceGroup.leases {
				secrets.set(leaseToFill, secret)
			}
			mu.Unlock()

			slog.Debug("secret lookup: fetched source",
				"mode", opts.Mode,
				"provider", sourceGroup.providerName,
				"account", sourceGroup.account,
				"source", sourceGroup.source,
				"lease_count", len(sourceGroup.leases))
			return nil
		})
	}

	waitErr := group.Wait()
	if waitErr != nil && !opts.ContinueOnError {
		return secrets, errs, waitErr
	}
	return secrets, errs, nil
}

func defaultProviderFactory(providerName, account string) (provider.SecretProvider, error) {
	return provider.DefaultRegistry().NewAdapter(providerName, account)
}

func defaultBatchPolicy(providerName, sourceURI string) bool {
	return provider.DefaultRegistry().CanBatch(providerName, sourceURI)
}

func legacyBatchPolicy(_ string, sourceURI string) bool {
	return strings.HasPrefix(strings.TrimSpace(sourceURI), "op://")
}

func defaultProviderNormalizer(providerName string) string {
	canonical, err := provider.DefaultRegistry().CanonicalName(providerName)
	if err != nil {
		return provider.NormalizeName(providerName)
	}
	return canonical
}

type plan struct {
	batches    []batch
	singletons []sourceGroup
}

type batch struct {
	providerName string
	account      string
	leases       []lease.Lease
}

type sourceGroup struct {
	providerName string
	account      string
	source       string
	leases       []lease.Lease
}

type providerAccount struct {
	providerName string
	account      string
}

type sourceKey struct {
	providerName string
	account      string
	source       string
}

type key sourceKey

func buildPlan(leases []lease.Lease, canBatch BatchPolicy, normalizeProvider ProviderNormalizer) plan {
	batchesByAccount := make(map[providerAccount]int)
	singletonsBySource := make(map[sourceKey]int)
	plan := plan{}

	for _, l := range leases {
		providerName := normalizeProvider(l.Provider)
		if canBatch(providerName, l.Source) {
			accountKey := providerAccount{providerName: providerName, account: l.OpAccount}
			idx, ok := batchesByAccount[accountKey]
			if !ok {
				idx = len(plan.batches)
				batchesByAccount[accountKey] = idx
				plan.batches = append(plan.batches, batch{
					providerName: providerName,
					account:      l.OpAccount,
				})
			}
			plan.batches[idx].leases = append(plan.batches[idx].leases, l)
			continue
		}

		singletonKey := sourceKey{providerName: providerName, account: l.OpAccount, source: l.Source}
		idx, ok := singletonsBySource[singletonKey]
		if !ok {
			idx = len(plan.singletons)
			singletonsBySource[singletonKey] = idx
			plan.singletons = append(plan.singletons, sourceGroup{
				providerName: providerName,
				account:      l.OpAccount,
				source:       l.Source,
			})
		}
		plan.singletons[idx].leases = append(plan.singletons[idx].leases, l)
	}

	return plan
}

func keyFor(l lease.Lease) key {
	return key{providerName: l.Provider, account: l.OpAccount, source: l.Source}
}

func errorsForLeases(leases []lease.Lease, err error) []Error {
	lookupErrs := make([]Error, 0, len(leases))
	for _, l := range leases {
		lookupErrs = append(lookupErrs, Error{Lease: l, Err: err})
	}
	return lookupErrs
}

func lookupErrors(providerErrs []provider.ProviderError) []Error {
	lookupErrs := make([]Error, 0, len(providerErrs))
	for _, providerErr := range providerErrs {
		lookupErrs = append(lookupErrs, Error{Lease: providerErr.Lease, Err: providerErr.Err})
	}
	return lookupErrs
}
