package provider

import (
	"encoding/json"
	"fmt"
	"os/exec"
	"strings"
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
	if strings.HasPrefix(sourceURI, "op+file://") {
		canonicalURI, err := p.resolveFileURI(sourceURI)
		if err != nil {
			return "", err
		}
		sourceURI = canonicalURI
	}

	return p.fetchWithRead(sourceURI)
}

// FetchBulk retrieves multiple secrets from 1Password using `op inject`.
func (p *OnePasswordCLI) FetchBulk(sources map[string]string) (map[string]string, error) {
	if len(sources) == 0 {
		return make(map[string]string), nil
	}

	template := ""
	for name, uri := range sources {
		template += fmt.Sprintf("%s=\"{{ %s }}\"\n", name, uri)
	}

	args := []string{"inject"}
	if p.Account != "" {
		args = append(args, "--account", p.Account)
	}
	cmd := cmdExecer.Command("op", args...)
	cmd.Stdin = strings.NewReader(template)

	output, err := cmd.Output()
	if err != nil {
		if exitErr, ok := err.(*exec.ExitError); ok {
			return nil, &OpError{
				Command:  "inject",
				ExitCode: exitErr.ExitCode(),
				Stderr:   string(exitErr.Stderr),
				Err:      err,
			}
		}
		return nil, fmt.Errorf("failed to execute 'op inject': %w", err)
	}

	secrets := make(map[string]string)
	for _, line := range strings.Split(string(output), "\n") {
		if parts := strings.SplitN(line, "=", 2); len(parts) == 2 {
			secrets[parts[0]] = strings.Trim(parts[1], "\"")
		}
	}

	return secrets, nil
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
