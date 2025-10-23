package provider

import (
	"errors"
	"os/exec"
	"strings"
	"sync"
	"testing"

	"github.com/mblarsen/env-lease/internal/config"
)

type mockExecer struct {
	CommandFunc func(name string, arg ...string) *exec.Cmd
}

func (m *mockExecer) Command(name string, arg ...string) *exec.Cmd {
	return m.CommandFunc(name, arg...)
}

func TestOnePasswordCLI_FetchLeases(t *testing.T) {
	originalExecer := cmdExecer
	defer func() { cmdExecer = originalExecer }()

	t.Run("groups leases by account", func(t *testing.T) {
		var capturedArgs [][]string
		var mu sync.Mutex

		var injectCall int
		cmdExecer = &mockExecer{
			CommandFunc: func(name string, arg ...string) *exec.Cmd {
				mu.Lock()
				capturedArgs = append(capturedArgs, arg)
				mu.Unlock()

				// Simulate the output of `op inject`
				if len(arg) > 0 && arg[0] == "inject" {
					injectCall++
					if injectCall == 1 {
						return exec.Command("echo", "-n", "lease_0=\"secret1\"")
					}
					return exec.Command("echo", "-n", "lease_0=\"secret2\"")
				}
				return exec.Command("echo", "-n", "my-secret")
			},
		}

		provider := &OnePasswordCLI{}
		leases := []config.Lease{
			{Variable: "VAR1", Source: "op://vault/item1", OpAccount: "account1"},
			{Variable: "VAR2", Source: "op://vault/item2", OpAccount: "account2"},
		}
		_, errs := provider.FetchLeases(leases)
		if len(errs) > 0 {
			t.Fatalf("unexpected errors: %v", errs)
		}

		if len(capturedArgs) != 2 {
			t.Fatalf("expected 2 calls to op, got %d", len(capturedArgs))
		}

		var foundAccount1, foundAccount2 bool
		for _, args := range capturedArgs {
			if strings.Contains(strings.Join(args, " "), "--account account1") {
				foundAccount1 = true
			}
			if strings.Contains(strings.Join(args, " "), "--account account2") {
				foundAccount2 = true
			}
		}

		if !foundAccount1 {
			t.Error("expected a call with --account account1")
		}
		if !foundAccount2 {
			t.Error("expected a call with --account account2")
		}
	})
}
func TestOnePasswordCLI_Fetch(t *testing.T) {
	originalExecer := cmdExecer
	defer func() { cmdExecer = originalExecer }()

	t.Run("successful fetch", func(t *testing.T) {
		cmdExecer = &mockExecer{
			CommandFunc: func(name string, arg ...string) *exec.Cmd {
				return exec.Command("echo", "-n", "my-secret")
			},
		}

		provider := &OnePasswordCLI{}
		secret, err := provider.Fetch("op://vault/item/field")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		if secret != "my-secret" {
			t.Fatalf("expected secret 'my-secret', got '%s'", secret)
		}
	})

	t.Run("command not found", func(t *testing.T) {
		cmdExecer = &mockExecer{
			CommandFunc: func(name string, arg ...string) *exec.Cmd {
				return exec.Command("command-that-does-not-exist")
			},
		}

		provider := &OnePasswordCLI{}
		_, err := provider.Fetch("op://vault/item/secret")
		if err == nil {
			t.Fatal("expected an error, got nil")
		}
	})

	t.Run("non-zero exit code", func(t *testing.T) {
		cmdExecer = &mockExecer{
			CommandFunc: func(name string, arg ...string) *exec.Cmd {
				return exec.Command("false") // `false` command always exits with 1
			},
		}

		provider := &OnePasswordCLI{}
		_, err := provider.Fetch("op://vault/item/secret")
		if err == nil {
			t.Fatal("expected an error, got nil")
		}
		var opErr *OpError
		if !errors.As(err, &opErr) {
			t.Fatalf("expected an *OpError, got %T", err)
		}
		if opErr.ExitCode != 1 {
			t.Errorf("expected exit code 1, got %d", opErr.ExitCode)
		}
	})

	t.Run("with account", func(t *testing.T) {
		var capturedArgs []string
		cmdExecer = &mockExecer{
			CommandFunc: func(name string, arg ...string) *exec.Cmd {
				capturedArgs = arg
				return exec.Command("echo", "my-secret")
			},
		}

		provider := &OnePasswordCLI{Account: "my-account"}
		_, err := provider.Fetch("op://vault/item/secret")
		if err != nil {
			t.Fatalf("expected no error, got %v", err)
		}

		expectedArgs := []string{"read", "op://vault/item/secret", "--account", "my-account"}
		if !equal(capturedArgs, expectedArgs) {
			t.Errorf("expected args %v, got %v", expectedArgs, capturedArgs)
		}
	})
}

func equal(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i, v := range a {
		if v != b[i] {
			return false
		}
	}
	return true
}
