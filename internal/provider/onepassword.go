package provider

import (
	"fmt"
	"os/exec"
	"strings"
)

// OnePasswordCLI is a SecretProvider that fetches secrets using the 1Password CLI.
type OnePasswordCLI struct {
	// Account is the 1Password account to use.
	Account string
}

// Fetch retrieves a secret from 1Password using the `op read` command.
func (p *OnePasswordCLI) Fetch(sourceURI string) (string, error) {
	args := []string{"read", sourceURI}
	if p.Account != "" {
		args = append(args, "--account", p.Account)
	}
	cmd := cmdExecer.Command("op", args...)
	output, err := cmd.Output()
	if err != nil {
		if exitErr, ok := err.(*exec.ExitError); ok {
			return "", &OpError{
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
	ExitCode int
	Stderr   string
	Err      error
}

func (e *OpError) Error() string {
	return fmt.Sprintf("op read failed with exit code %d: %s", e.ExitCode, e.Stderr)
}

func (e *OpError) Unwrap() error {
	return e.Err
}
