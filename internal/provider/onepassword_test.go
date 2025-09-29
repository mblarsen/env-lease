package provider

import (
	"errors"
	"os/exec"
	"testing"
)

type mockExecer struct {
	CommandFunc func(name string, arg ...string) *exec.Cmd
}

func (m *mockExecer) Command(name string, arg ...string) *exec.Cmd {
	return m.CommandFunc(name, arg...)
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
