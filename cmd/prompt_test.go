package cmd

import (
	"bytes"
	"io"
	"os"
	"testing"
)

func TestConfirm(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  bool
	}{
		{"yes", "y\n", true},
		{"YES", "Y\n", true},
		{"Yes", "Yes\n", true},
		{"no", "n\n", false},
		{"NO", "N\n", false},
		{"No", "No\n", false},
		{"empty", "\n", false},
		{"random", "foo\n", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			oldStdin := os.Stdin
			defer func() { os.Stdin = oldStdin }()

			r, w, err := os.Pipe()
			if err != nil {
				t.Fatal(err)
			}
			defer r.Close()
			defer w.Close()

			_, err = w.WriteString(tt.input)
			if err != nil {
				t.Fatal(err)
			}
			w.Close()

			os.Stdin = r

			oldStderr := os.Stderr
			defer func() { os.Stderr = oldStderr }()
			devNull, err := os.OpenFile(os.DevNull, os.O_WRONLY, 0666)
			if err != nil {
				t.Fatal(err)
			}
			defer devNull.Close()
			os.Stderr = devNull

			if got := confirm("test prompt"); got != tt.want {
				t.Errorf("confirm() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestConfirm_input_yes_and_no(t *testing.T) {
	// Redirect stdin
	oldStdin := os.Stdin
	r, w, _ := os.Pipe()
	os.Stdin = r
	defer func() {
		os.Stdin = oldStdin
		w.Close()
		r.Close()
	}()

	// Redirect stderr to capture prompt
	oldStderr := os.Stderr
	re, we, _ := os.Pipe()
	os.Stderr = we
	defer func() {
		os.Stderr = oldStderr
		we.Close()
		re.Close()
	}()

	// Test "y"
	go func() {
		if _, err := w.WriteString("y\n"); err != nil {
			t.Errorf("failed to write to pipe: %v", err)
		}
	}()
	if !confirm("Grant") {
		t.Error("expected true for 'y'")
	}

	// Test "n"
	go func() {
		if _, err := w.WriteString("n\n"); err != nil {
			t.Errorf("failed to write to pipe: %v", err)
		}
	}()
	if confirm("Grant") {
		t.Error("expected false for 'n'")
	}

	// Test "yes"
	go func() {
		if _, err := w.WriteString("yes\n"); err != nil {
			t.Errorf("failed to write to pipe: %v", err)
		}
	}()
	if !confirm("Grant") {
		t.Error("expected true for 'yes'")
	}

	// Test ""
	go func() {
		if _, err := w.WriteString("\n"); err != nil {
			t.Errorf("failed to write to pipe: %v", err)
		}
	}()
	if confirm("Grant") {
		t.Error("expected false for ''")
	}

	we.Close()
	var buf bytes.Buffer
	if _, err := io.Copy(&buf, re); err != nil {
		t.Errorf("failed to read from pipe: %v", err)
	}
	if !bytes.Contains(buf.Bytes(), []byte("Grant [y/N] ")) {
		t.Errorf("expected prompt 'Grant [y/N] ', got '%s'", buf.String())
	}
}
