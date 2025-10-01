package transform

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestPipeline(t *testing.T) {
	tests := []struct {
		name            string
		transformations []string
		input           string
		expected        string
		expectErr       bool
	}{
		{
			name:            "base64 encode",
			transformations: []string{"base64-encode"},
			input:           "hello",
			expected:        "aGVsbG8=",
			expectErr:       false,
		},
		{
			name:            "base64 decode",
			transformations: []string{"base64-decode"},
			input:           "aGVsbG8=",
			expected:        "hello",
			expectErr:       false,
		},
		{
			name:            "json select",
			transformations: []string{"json", "select 'data.key'"},
			input:           `{"data": {"key": "value"}}`,
			expected:        "value",
			expectErr:       false,
		},
		{
			name:            "toml select",
			transformations: []string{"toml", "select 'data.key'"},
			input: `[data]
key = "value"`,
			expected:  "value",
			expectErr: false,
		},
		{
			name:            "yaml select",
			transformations: []string{"yaml", "select 'data.key'"},
			input: `data:
  key: value`,
			expected:  "value",
			expectErr: false,
		},
		{
			name:            "complex pipeline",
			transformations: []string{"base64-decode", "json", "select 'data.key'"},
			input:           "eyJkYXRhIjogeyJrZXkiOiAidmFsdWUifX0=",
			expected:        "value",
			expectErr:       false,
		},
		{
			name:            "invalid transformer",
			transformations: []string{"invalid"},
			input:           "hello",
			expected:        "",
			expectErr:       true,
		},
		{
			name:            "to_json",
			transformations: []string{"json", "select 'data'", "to_json"},
			input:           `{"data": {"key": "value"}}`,
			expected: `{
  "key": "value"
}`,
			expectErr: false,
		},
		{
			name:            "to_yaml",
			transformations: []string{"json", "select 'data'", "to_yaml"},
			input:           `{"data": {"key": "value"}}`,
			expected:        "key: value\n",
			expectErr:       false,
		},
		{
			name:            "to_toml",
			transformations: []string{"json", "select 'data'", "to_toml"},
			input:           `{"data": {"key": "value"}}`,
			expected:        "key = \"value\"\n",
			expectErr:       false,
		},
		{
			name:            "invalid pipeline",
			transformations: []string{"json", "base64-encode"},
			input:           `{"data": {"key": "value"}}`,
			expected:        "",
			expectErr:       true, // Expect error on Run(), not NewPipeline()
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			pipeline, err := NewPipeline(tt.transformations)
			if err != nil && tt.name != "invalid transformer" {
				t.Fatalf("NewPipeline() error = %v, expectErr %v", err, tt.expectErr)
			}
			if tt.name == "invalid transformer" {
				if err == nil {
					t.Fatalf("NewPipeline() expected error, got nil")
				}
				return
			}

			output, err := pipeline.Run(tt.input)
			if (err != nil) != tt.expectErr {
				t.Fatalf("Run() error = %v, expectErr %v", err, tt.expectErr)
			}
			if err != nil {
				return
			}

			if output != tt.expected {
				t.Errorf("Run() = %v, want %v", output, tt.expected)
			}
		})
	}
}

func TestExplodeTransformerWithArgs(t *testing.T) {
	tests := []struct {
		name           string
		transformation string
		input          string
		expected       ExplodedData
		expectErr      bool
		errContains    string
	}{
		{
			name:           "no args",
			transformation: "explode",
			input:          `{"KEY1": "v1", "KEY2": "v2"}`,
			expected:       ExplodedData{"KEY1": "v1", "KEY2": "v2"},
		},
		{
			name:           "filter only",
			transformation: "explode(filter=ORY_)",
			input:          `{"ORY_KEY": "v1", "OTHER_KEY": "v2"}`,
			expected:       ExplodedData{"ORY_KEY": "v1"},
		},
		{
			name:           "prefix only",
			transformation: "explode(prefix=REACT_)",
			input:          `{"KEY1": "v1", "KEY2": "v2"}`,
			expected:       ExplodedData{"REACT_KEY1": "v1", "REACT_KEY2": "v2"},
		},
		{
			name:           "filter and prefix",
			transformation: "explode(filter=ORY_, prefix=REACT_)",
			input:          `{"ORY_KEY": "v1", "OTHER_KEY": "v2"}`,
			expected:       ExplodedData{"REACT_ORY_KEY": "v1"},
		},
		{
			name:           "prefix and filter (reversed order)",
			transformation: "explode(prefix=REACT_, filter=ORY_)",
			input:          `{"ORY_KEY": "v1", "OTHER_KEY": "v2"}`,
			expected:       ExplodedData{"REACT_ORY_KEY": "v1"},
		},
		{
			name:           "filter is case-insensitive",
			transformation: "explode(filter=ory_)",
			input:          `{"ORY_KEY": "v1", "OTHER_KEY": "v2"}`,
			expected:       ExplodedData{"ORY_KEY": "v1"},
		},
		{
			name:           "invalid argument",
			transformation: "explode(foo=bar)",
			input:          `{}`,
			expectErr:      true,
			errContains:    "unknown argument",
		},
		{
			name:           "malformed argument",
			transformation: "explode(filter=)",
			input:          `{}`,
			expectErr:      true,
			errContains:    "empty value for argument",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			pipeline, err := NewPipeline([]string{"json", tt.transformation})
			if tt.expectErr {
				require.Error(t, err)
				if tt.errContains != "" {
					require.Contains(t, err.Error(), tt.errContains)
				}
				return
			}
			require.NoError(t, err)

			output, err := pipeline.Run(tt.input)
			require.NoError(t, err)
			exploded, ok := output.(ExplodedData)
			require.True(t, ok)
			assert.Equal(t, tt.expected, exploded)
		})
	}
}
