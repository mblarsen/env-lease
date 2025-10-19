package cmd

import (
	"bytes"
	"strings"
	"testing"
)

func TestDoConfirm(t *testing.T) {
	tests := []struct {
		name       string
		inputs     []string // Each string is a separate line of input for a single prompt.
		prompts    []string // The prompts to display.
		expected   []bool   // The expected return values from confirm.
		finalState interactiveState
	}{
		{
			name:       "Yes",
			inputs:     []string{"y\n"},
			prompts:    []string{"Prompt 1?"},
			expected:   []bool{true},
			finalState: stateAsk,
		},
		{
			name:       "No",
			inputs:     []string{"n\n"},
			prompts:    []string{"Prompt 1?"},
			expected:   []bool{false},
			finalState: stateAsk,
		},
		{
			name:       "Default to No",
			inputs:     []string{"\n"},
			prompts:    []string{"Prompt 1?"},
			expected:   []bool{false},
			finalState: stateAsk,
		},
		{
			name:       "All",
			inputs:     []string{"a\n"},
			prompts:    []string{"Prompt 1?"},
			expected:   []bool{true},
			finalState: stateAlways,
		},
		{
			name:       "Deny",
			inputs:     []string{"d\n"},
			prompts:    []string{"Prompt 1?"},
			expected:   []bool{false},
			finalState: stateDeny,
		},
		{
			name:       "Help then Yes",
			inputs:     []string{"?\n", "y\n"},
			prompts:    []string{"Prompt 1?"},
			expected:   []bool{true},
			finalState: stateAsk,
		},
		{
			name:       "Invalid then No",
			inputs:     []string{"x\n", "n\n"},
			prompts:    []string{"Prompt 1?"},
			expected:   []bool{false},
			finalState: stateAsk,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			resetConfirmState()
			input := strings.Join(tt.inputs, "")
			in := bytes.NewBufferString(input)

			for i, prompt := range tt.prompts {
				result := doConfirm(prompt, in)
				if i < len(tt.expected) && result != tt.expected[i] {
					t.Errorf("doConfirm() for prompt '%s' got %v, want %v", prompt, result, tt.expected[i])
				}
			}

			if confirmState != tt.finalState {
				t.Errorf("final confirmState got %v, want %v", confirmState, tt.finalState)
			}
		})
	}

	t.Run("All then next is auto-yes", func(t *testing.T) {
		resetConfirmState()
		in := bytes.NewBufferString("a\n")
		_ = doConfirm("P1?", in)
		if confirmState != stateAlways {
			t.Fatalf("confirmState after 'a' got %v, want %v", confirmState, stateAlways)
		}
		result := doConfirm("P2?", in)
		if !result {
			t.Errorf("doConfirm() for prompt 'P2?' got %v, want %v", result, true)
		}
	})

	t.Run("Deny then next is auto-no", func(t *testing.T) {
		resetConfirmState()
		in := bytes.NewBufferString("d\n")
		_ = doConfirm("P1?", in)
		if confirmState != stateDeny {
			t.Fatalf("confirmState after 'd' got %v, want %v", confirmState, stateDeny)
		}
		result := doConfirm("P2?", in)
		if result {
			t.Errorf("doConfirm() for prompt 'P2?' got %v, want %v", result, false)
		}
	})

	t.Run("Yes then All", func(t *testing.T) {
		resetConfirmState()
		inY := bytes.NewBufferString("y\n")
		inA := bytes.NewBufferString("a\n")

		resultY := doConfirm("P1?", inY)
		if !resultY {
			t.Errorf("doConfirm() for 'y' got %v, want %v", resultY, true)
		}
		if confirmState != stateAsk {
			t.Errorf("confirmState after 'y' got %v, want %v", confirmState, stateAsk)
		}

		resultA := doConfirm("P2?", inA)
		if !resultA {
			t.Errorf("doConfirm() for 'a' got %v, want %v", resultA, true)
		}
		if confirmState != stateAlways {
			t.Errorf("confirmState after 'a' got %v, want %v", confirmState, stateAlways)
		}
	})
}
