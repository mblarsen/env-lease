package provider

import (
	"encoding/json"
	"fmt"
	"log/slog"
	"os/exec"
	"strings"

	"github.com/mblarsen/env-lease/internal/config"
)

// OnePasswordCLI is a SecretProvider that fetches secrets using the 1Password CLI.
type OnePasswordCLI struct {
	// Account is the 1Password account to use.
	Account string
}

// opItem represents the structure of a 1Password item from `op item get`.
type opItem struct {
	ID    string `json:"id"`
	Vault struct {
		ID string `json:"id"`
	} `json:"vault"`
	Files []opFile `json:"files"`
}

// opFile represents a file within a 1Password item.
type opFile struct {
	ID   string `json:"id"`
	Name string `json:"name"`
}

// sanitizeOpURI defensively trims quotes/space and drops any trailing '=' run.
// This guards against cases where a revoke/format string like "%s=" leaks into
// a later op:// field path. Canonical 1Password field paths never end with '='.
func sanitizeOpURI(u string) string {
	u = strings.TrimSpace(u)
	u = strings.Trim(u, "\"")
	u = strings.TrimRight(u, "=")
	return u
}

// resolveFileURI resolves a friendly `op+file://<item-name>/<file-name>` URI
// to a canonical `op://<vault-id>/<item-id>/<file-id>` URI.
func (p *OnePasswordCLI) resolveFileURI(sourceURI string) (string, error) {
	parts := strings.SplitN(strings.TrimPrefix(sourceURI, "op+file://"), "/", 2)
	if len(parts) != 2 {
		return "", fmt.Errorf("invalid op+file URI format: %s", sourceURI)
	}
	itemName, fileName := parts[0], parts[1]

	args := []string{"item", "get", itemName, "--format", "json"}
	if p.Account != "" {
		args = append(args, "--account", p.Account)
	}
	cmd := cmdExecer.Command("op", args...)
	output, err := cmd.Output()
	if err != nil {
		if exitErr, ok := err.(*exec.ExitError); ok {
			return "", &OpError{
				Command:  "item get",
				ExitCode: exitErr.ExitCode(),
				Stderr:   string(exitErr.Stderr),
				Err:      err,
			}
		}
		return "", fmt.Errorf("failed to execute 'op item get': %w", err)
	}

	var item opItem
	if err := json.Unmarshal(output, &item); err != nil {
		return "", fmt.Errorf("failed to parse 'op item get' output: %w", err)
	}

	for _, file := range item.Files {
		if file.Name == fileName {
			return fmt.Sprintf("op://%s/%s/%s", item.Vault.ID, item.ID, file.ID), nil
		}
	}

	return "", fmt.Errorf("file '%s' not found in item '%s'", fileName, itemName)
}

// Fetch retrieves a secret from 1Password. It supports `op://` URIs for secrets
// and documents, and `op+file://` for a user-friendly way to reference documents.
func (p *OnePasswordCLI) Fetch(sourceURI string) (string, error) {
	uri := sanitizeOpURI(sourceURI)
	if strings.HasPrefix(uri, "op+file://") {
		r, err := p.resolveFileURI(uri)
		if err != nil {
			return "", err
		}
		uri = r // canonical op://.../.../...
	}
	args := []string{"read", uri}
	if p.Account != "" {
		args = append(args, "--account", p.Account)
	}
	out, err := cmdExecer.Command("op", args...).Output()
	if err != nil {
		if exitErr, ok := err.(*exec.ExitError); ok {
			return "", &OpError{Command: "read", ExitCode: exitErr.ExitCode(), Stderr: string(exitErr.Stderr), Err: err}
		}
		return "", fmt.Errorf("failed to execute 'op read': %w", err)
	}
	return strings.TrimRight(string(out), "\n"), nil
}

func (p *OnePasswordCLI) FetchBulk(sources map[string]string) (map[string]string, error) {
	// Build a template that will echo NAME=VALUE per line.
	// Use generated safe identifiers as placeholders and map them back to the original names.
	var (
		b          strings.Builder
		index      int
		nameBySafe = make(map[string]string, len(sources))
	)
	for name, uri := range sources {
		// uri MUST be canonical op://...; sanitize to avoid trailing '=' or quotes
		uri = sanitizeOpURI(uri)
		safe := fmt.Sprintf("lease_%d", index)
		index++
		nameBySafe[safe] = name
		fmt.Fprintf(&b, "%s={{ %s }}\n", safe, uri)
	}
	slog.Debug("onepassword: inject template", "template", b.String(), "mapping", nameBySafe)
	// op inject reads template from stdin and prints expanded result
	args := []string{"inject"}
	if p.Account != "" {
		args = append(args, "--account", p.Account)
	}
	cmd := cmdExecer.Command("op", args...)
	cmd.Stdin = strings.NewReader(b.String())
	out, err := cmd.Output()
	if err != nil {
		if exitErr, ok := err.(*exec.ExitError); ok {
			return nil, &OpError{Command: "inject", ExitCode: exitErr.ExitCode(), Stderr: string(exitErr.Stderr), Err: err}
		}
		return nil, fmt.Errorf("failed to execute 'op inject': %w", err)
	}

	results := make(map[string]string, len(sources))
	for line := range strings.SplitSeq(strings.TrimRight(string(out), "\n"), "\n") {
		if line == "" {
			continue
		}
		kv := strings.SplitN(line, "=", 2)
		if len(kv) != 2 {
			return nil, fmt.Errorf("unexpected output from 'op inject': %q", line)
		}
		name, val := kv[0], kv[1]
		orig, ok := nameBySafe[name]
		if !ok {
			return nil, fmt.Errorf("unexpected placeholder from 'op inject': %q", name)
		}
		results[orig] = val
	}
	return results, nil
}

// fetchWithRead retrieves a secret from 1Password using the `op read` command.
func (p *OnePasswordCLI) fetchWithRead(sourceURI string) (string, error) {
	args := []string{"read", sourceURI}
	if p.Account != "" {
		args = append(args, "--account", p.Account)
	}
	cmd := cmdExecer.Command("op", args...)
	output, err := cmd.Output()
	if err != nil {
		if exitErr, ok := err.(*exec.ExitError); ok {
			return "", &OpError{
				Command:  "read",
				ExitCode: exitErr.ExitCode(),
				Stderr:   string(exitErr.Stderr),
				Err:      err,
			}
		}
		return "", fmt.Errorf("failed to execute 'op read': %w", err)
	}

	if len(output) == 0 {
		return "", fmt.Errorf("op read returned empty output for %s", sourceURI)
	}

	return strings.TrimSpace(string(output)), nil
}

// FetchLeases fetches secrets for a slice of leases, using `op inject` for op://
// URIs and falling back to individual `op read` calls for op+file:// URIs.
func (p *OnePasswordCLI) FetchLeases(leases []config.Lease) (map[string]string, []ProviderError) {
	secrets := make(map[string]string, len(leases))
	var perrs []ProviderError

	// Partition by scheme
	type leaseBatch map[string][]config.Lease // sanitized source -> leases sharing it
	type accountBatch struct {
		account string
		leases  leaseBatch
	}
	opAccounts := map[string]*accountBatch{} // grouping key -> batch

	var singletons []config.Lease // op+file and anything non-batchable

	for _, l := range leases {
		src := sanitizeOpURI(l.Source)
		if strings.HasPrefix(src, "op://") {
			actualAcct := l.OpAccount
			if actualAcct == "" {
				actualAcct = p.Account
			}
			key := actualAcct
			if key == "" {
				key = "default"
			}

			group := opAccounts[key]
			if group == nil {
				group = &accountBatch{
					account: actualAcct,
					leases:  make(leaseBatch),
				}
				opAccounts[key] = group
			}
			group.leases[src] = append(group.leases[src], l)
			slog.Debug("onepassword: queued op lease",
				"account_group", key,
				"account", actualAcct,
				"source", l.Source,
				"sanitized", src)
			continue
		}
		// op+file:// and others are fetched one-by-one (grant-side also caches)
		singletons = append(singletons, l)
		slog.Debug("onepassword: queued singleton lease",
			"source", l.Source,
			"sanitized", src)
	}

	// Batch op:// by account
	for key, batch := range opAccounts {
		sub := &OnePasswordCLI{Account: batch.account}
		request := make(map[string]string, len(batch.leases))
		for sanitized := range batch.leases {
			request[sanitized] = sanitized
		}

		slog.Debug("onepassword: fetch bulk start",
			"account_group", key,
			"account", batch.account,
			"request_count", len(request))

		res, err := sub.FetchBulk(request)
		if err != nil {
			// attribute an error to each lease in this batch
			for _, leases := range batch.leases {
				for _, l := range leases {
					perrs = append(perrs, ProviderError{Lease: l, Err: err})
				}
			}
			slog.Debug("onepassword: fetch bulk error",
				"account_group", key,
				"account", batch.account,
				"err", err)
			continue
		}

		for sanitized, leases := range batch.leases {
			val, ok := res[sanitized]
			if !ok {
				continue
			}
			for _, l := range leases {
				secrets[l.Source] = val
				slog.Debug("onepassword: fetched lease",
					"account_group", key,
					"account", batch.account,
					"source", l.Source,
					"sanitized", sanitized)
			}
		}
	}

	// Fetch singletons
	for _, l := range singletons {
		slog.Debug("onepassword: fetch singleton start", "source", l.Source)
		val, err := p.Fetch(l.Source)
		if err != nil {
			perrs = append(perrs, ProviderError{Lease: l, Err: err})
			slog.Debug("onepassword: fetch singleton error",
				"source", l.Source,
				"err", err)
			continue
		}
		secrets[l.Source] = val
		slog.Debug("onepassword: fetch singleton complete", "source", l.Source)
	}

	return secrets, perrs
}

// OpError is a custom error for 1Password CLI errors.
type OpError struct {
	Command  string
	ExitCode int
	Stderr   string
	Err      error
}

func (e *OpError) Error() string {
	return fmt.Sprintf("op %s failed with exit code %d: %s", e.Command, e.ExitCode, e.Stderr)
}

func (e *OpError) Unwrap() error {
	return e.Err
}
