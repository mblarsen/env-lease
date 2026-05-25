// Package grantflow owns Grant workflow orchestration.
package grantflow

import (
	"fmt"
	"log/slog"
	"path/filepath"
	"strings"
	"time"

	"github.com/mblarsen/env-lease/internal/ipc"
	"github.com/mblarsen/env-lease/internal/lease"
	"github.com/mblarsen/env-lease/internal/secretlookup"
	"github.com/mblarsen/env-lease/internal/transform"
)

// ConfirmFunc asks the user whether a Grant step should proceed.
type ConfirmFunc func(prompt string) bool

// NoticeFunc reports non-secret workflow messages to the caller.
type NoticeFunc func(message string)

// MaterializeFunc materializes one transformed Secret and returns the Lease data
// that should be registered with the Daemon.
type MaterializeFunc func(l lease.Lease, secret string) (Materialized, error)

// Materialized is the observable output of materializing one Secret.
type Materialized struct {
	Leases        []ipc.Lease
	ShellCommands []string
}

// Options controls Grant workflow behaviour.
type Options struct {
	Interactive     bool
	ContinueOnError bool
	Append          bool
	Override        bool
}

// Result is the final output of a Grant workflow before IPC delivery.
type Result struct {
	Request       ipc.GrantRequest
	ShellCommands []string
	Noop          bool
}

// Flow runs Grant workflows for a normalized Lease set.
type Flow struct {
	Confirm     ConfirmFunc
	Notice      NoticeFunc
	Materialize MaterializeFunc
	Lookup      *secretlookup.Lookup
}

// Run executes either the non-interactive or interactive Grant workflow.
func (f Flow) Run(set *lease.Set, opts Options) (Result, error) {
	if set == nil {
		return Result{}, fmt.Errorf("lease set is nil")
	}
	if f.Materialize == nil {
		return Result{}, fmt.Errorf("materializer is nil")
	}
	if f.Lookup == nil {
		f.Lookup = secretlookup.New()
	}
	if f.Notice == nil {
		f.Notice = func(string) {}
	}

	var result Result
	var err error
	if opts.Interactive {
		result, err = f.runInteractive(set, opts)
	} else {
		result, err = f.runNonInteractive(set, opts)
	}
	if err != nil {
		return Result{}, err
	}
	if result.Noop {
		return result, nil
	}

	result.Request = ipc.GrantRequest{
		Command:    "grant",
		Leases:     result.Request.Leases,
		Override:   opts.Override,
		Append:     opts.Append,
		ConfigFile: set.ConfigFile,
	}
	return result, nil
}

func (f Flow) runNonInteractive(set *lease.Set, opts Options) (Result, error) {
	var errs []Error
	var result Result

	fetched, lookupErrs, fetchErr := f.Lookup.Fetch(set.Leases, secretlookup.Options{ContinueOnError: opts.ContinueOnError, Mode: "non-interactive"})
	errs = append(errs, lookupGrantErrors(lookupErrs)...)
	if fetchErr != nil {
		return Result{}, Errors{errs: errs}
	}

	for _, l := range set.Leases {
		raw, ok := fetched.Get(l)
		if !ok {
			if !hasErrorForSource(errs, l.Source) {
				errs = append(errs, Error{Source: l.Source, Err: fmt.Errorf("no secret returned")})
			}
			if !opts.ContinueOnError {
				return Result{}, Errors{errs: errs}
			}
			continue
		}

		materialized, err := f.applyAndMaterialize(l, raw)
		if err != nil {
			errs = append(errs, Error{Source: l.Source, Err: err})
			if !opts.ContinueOnError {
				return Result{}, Errors{errs: errs}
			}
			continue
		}
		result.append(materialized)
	}

	if len(errs) > 0 {
		return Result{}, Errors{errs: errs}
	}
	return result, nil
}

func (f Flow) runInteractive(set *lease.Set, opts Options) (Result, error) {
	slog.Debug("interactive grant: phase 1 start", "lease_count", len(set.Leases))
	selectedLeases := f.approveSources(set.Leases)
	if len(selectedLeases) == 0 {
		f.Notice("No leases selected.")
		return Result{Noop: true}, nil
	}

	slog.Debug("interactive grant: phase 1 approvals",
		"selected_count", len(selectedLeases),
		"skipped_count", len(set.Leases)-len(selectedLeases))

	if opts.Append {
		f.Notice("Append mode enabled: skipped leases will remain unchanged.")
	}

	var errs []Error
	fetched, lookupErrs, fetchErr := f.Lookup.Fetch(selectedLeases, secretlookup.Options{ContinueOnError: opts.ContinueOnError, Mode: "interactive"})
	errs = append(errs, lookupGrantErrors(lookupErrs)...)
	if fetchErr != nil {
		return Result{}, Errors{errs: errs}
	}

	prepared, prepareErrs := f.prepareInteractive(selectedLeases, fetched, opts)
	errs = append(errs, prepareErrs...)
	if len(prepareErrs) > 0 && !opts.ContinueOnError {
		return Result{}, Errors{errs: errs}
	}

	result, materializeErrs := f.materializeInteractive(prepared, opts)
	errs = append(errs, materializeErrs...)
	if len(errs) > 0 && !opts.ContinueOnError {
		return Result{}, Errors{errs: errs}
	}
	return result, nil
}

func (f Flow) approveSources(leases []lease.Lease) []lease.Lease {
	selected := make([]lease.Lease, 0, len(leases))
	for _, l := range leases {
		if f.confirm(promptForSource(l)) {
			selected = append(selected, l)
		}
	}
	return selected
}

type preparedInteractive struct {
	simpleApproved   []transform.Secret
	explodedChildren []transform.Secret
	parentApproved   []ipc.Lease
}

func (f Flow) prepareInteractive(selected []lease.Lease, fetched secretlookup.Secrets, opts Options) (preparedInteractive, []Error) {
	prepared := preparedInteractive{}
	var errs []Error

	slog.Debug("interactive grant: preprocessing transforms", "selected_count", len(selected))
	for _, l := range selected {
		raw, ok := fetched.Get(l)
		if !ok {
			errs = append(errs, Error{Source: l.Source, Err: fmt.Errorf("no secret returned")})
			if !opts.ContinueOnError {
				return prepared, errs
			}
			continue
		}

		slog.Debug("interactive grant: preparing lease",
			"source", l.Source,
			"explode", l.IsExplode(),
			"transform_steps", len(l.Transform))

		transformResult, err := applyTransform(l, raw)
		if err != nil {
			errs = append(errs, Error{Source: l.Source, Err: err})
			if !opts.ContinueOnError {
				return prepared, errs
			}
			continue
		}

		if transformResult.IsExploded() {
			slog.Debug("interactive grant: explode result", "source", l.Source, "child_count", len(transformResult.Secrets))
			parent, err := f.Materialize(*transformResult.Parent, "")
			if err != nil {
				errs = append(errs, Error{Source: l.Source, Err: err})
				if !opts.ContinueOnError {
					return prepared, errs
				}
				continue
			}
			if len(parent.Leases) == 0 {
				errs = append(errs, Error{Source: l.Source, Err: fmt.Errorf("explode parent produced no leases")})
				if !opts.ContinueOnError {
					return prepared, errs
				}
				continue
			}
			prepared.parentApproved = append(prepared.parentApproved, parent.Leases...)
			prepared.explodedChildren = append(prepared.explodedChildren, transformResult.Secrets...)
			continue
		}

		for _, transformedSecret := range transformResult.Secrets {
			slog.Debug("interactive grant: pipeline produced single value", "source", transformedSecret.Lease.Source)
			prepared.simpleApproved = append(prepared.simpleApproved, transformedSecret)
		}
	}

	return prepared, errs
}

func (f Flow) materializeInteractive(prepared preparedInteractive, opts Options) (Result, []Error) {
	slog.Debug("interactive grant: phase 3 start",
		"simple_count", len(prepared.simpleApproved),
		"explode_children", len(prepared.explodedChildren))

	var result Result
	var errs []Error
	result.Request.Leases = append(result.Request.Leases, prepared.parentApproved...)

	for _, secret := range prepared.simpleApproved {
		materialized, err := f.Materialize(secret.Lease, secret.Value)
		if err != nil {
			errs = append(errs, Error{Source: secret.Lease.Source, Err: err})
			if !opts.ContinueOnError {
				return result, errs
			}
			continue
		}
		result.append(materialized)
	}

	for _, secret := range prepared.explodedChildren {
		if !f.confirm(fmt.Sprintf("Grant lease for '%s'?", secret.Lease.Variable)) {
			continue
		}
		materialized, err := f.Materialize(secret.Lease, secret.Value)
		if err != nil {
			errs = append(errs, Error{Source: secret.Lease.Source, Err: err})
			if !opts.ContinueOnError {
				return result, errs
			}
			continue
		}
		result.append(materialized)
	}

	return result, errs
}

func (f Flow) applyAndMaterialize(l lease.Lease, raw string) (Materialized, error) {
	warnLongLease(l)
	transformResult, err := applyTransform(l, raw)
	if err != nil {
		return Materialized{}, fmt.Errorf("failed to transform secret: %w", err)
	}

	var materialized Materialized
	if transformResult.IsExploded() {
		f.Notice(fmt.Sprintf("Granting sub-leases from '%s'%s:", l.Source, transformSummary(l.Transform)))
		parent, err := f.Materialize(*transformResult.Parent, "")
		if err != nil {
			return Materialized{}, err
		}
		materialized.append(parent)
	}

	for _, transformedSecret := range transformResult.Secrets {
		m, err := f.Materialize(transformedSecret.Lease, transformedSecret.Value)
		if err != nil {
			return Materialized{}, err
		}
		materialized.append(m)
	}
	return materialized, nil
}

func applyTransform(l lease.Lease, raw string) (transform.Result, error) {
	return transform.Apply(l, raw)
}

func (f Flow) confirm(prompt string) bool {
	if f.Confirm == nil {
		return false
	}
	return f.Confirm(prompt)
}

func (r *Result) append(materialized Materialized) {
	r.Request.Leases = append(r.Request.Leases, materialized.Leases...)
	r.ShellCommands = append(r.ShellCommands, materialized.ShellCommands...)
}

func (m *Materialized) append(other Materialized) {
	m.Leases = append(m.Leases, other.Leases...)
	m.ShellCommands = append(m.ShellCommands, other.ShellCommands...)
}

func promptForSource(l lease.Lease) string {
	var key string
	if l.IsExplode() {
		key = fmt.Sprintf("leases from '%s'%s", l.Source, transformSummary(l.Transform))
	} else if l.Variable != "" {
		key = fmt.Sprintf("'%s'", l.Variable)
	} else {
		key = fmt.Sprintf("'%s'", l.Source)
	}
	return fmt.Sprintf("Grant %s?", key)
}

func transformSummary(transforms []string) string {
	if len(transforms) == 0 {
		return ""
	}
	return fmt.Sprintf(" (%s)", strings.Join(transforms, ", "))
}

func warnLongLease(l lease.Lease) {
	duration, err := time.ParseDuration(l.Duration)
	if err == nil && duration > 12*time.Hour {
		slog.Warn("Leases longer than 12 hours are discouraged for security reasons.")
	}
}

func lookupGrantErrors(lookupErrs []secretlookup.Error) []Error {
	errs := make([]Error, 0, len(lookupErrs))
	for _, lookupErr := range lookupErrs {
		errs = append(errs, Error{Source: lookupErr.Lease.Source, Err: lookupErr.Err})
	}
	return errs
}

func hasErrorForSource(errs []Error, source string) bool {
	for _, err := range errs {
		if err.Source == source {
			return true
		}
	}
	return false
}

// NeedsDirenv reports whether a Grant result touched a .envrc destination.
func (r Result) NeedsDirenv() bool {
	for _, l := range r.Request.Leases {
		if filepath.Base(l.Destination) == ".envrc" {
			return true
		}
	}
	return false
}
