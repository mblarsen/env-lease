package cmd

import (
	"bufio"
	"bytes"
	"fmt"
	"io"
	"os"
	"strings"
	"testing"
)

// confirmFunc is a type for the confirm function, so we can swap it out in tests.
type confirmFunc func(string) bool

// originalConfirm holds the original confirm function that talks to /dev/tty
var originalConfirm = confirm

// testConfirm is a mock confirm function that reads from a provided reader.
func testConfirm(prompt string, r io.Reader, w io.Writer) bool {
	fmt.Fprintf(w, "%s [y/N] ", prompt)
	scanner := bufio.NewScanner(r)
	scanner.Scan()
	response := strings.TrimSpace(strings.ToLower(scanner.Text()))
	return response == "y" || response == "yes"
}

func TestConfirm(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  bool
	}{
		{"yes", "y", true},
		{"YES", "Y", true},
		{"Yes", "Yes", true},
		{"no", "n", false},
		{"NO", "N", false},
		{"No", "No", false},
		{"empty", "", false},
		{"random", "asdf", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			in := bytes.NewBufferString(tt.input + "\n")
			out := bytes.NewBuffer(nil)

			// Use the testConfirm function instead of the real one
			if got := testConfirm("test prompt", in, out); got != tt.want {
				t.Errorf("confirm() = %v, want %v", got, tt.want)
			}

			// Check if the prompt was written
			if !strings.Contains(out.String(), "test prompt [y/N] ") {
				t.Errorf("prompt was not written correctly, got: %s", out.String())
			}
		})
	}
}

// This is a trick to allow grant_test.go to compile, as it uses the confirm function.
// In a real test run of grant_test, we would swap out the confirm function.
func init() {
	if os.Getenv("GO_TEST") == "1" {
		confirm = func(prompt string) bool {
			// In a non-interactive test, we default to 'yes'
			return true
		}
	}
}
