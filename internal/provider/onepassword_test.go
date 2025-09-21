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
				return exec.Command("echo", "my-secret")
			},
		}

		provider := &OnePasswordCLI{}
		secret, err := provider.Fetch("op://vault/item/secret")
		if err != nil {
			t.Fatalf("expected no error, got %v", err)
		}
		if secret != "my-secret\n" {
			t.Errorf("expected secret 'my-secret\\n', got '%s'", secret)
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
}
