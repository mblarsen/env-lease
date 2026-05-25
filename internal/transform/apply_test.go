package transform

import (
	"testing"

	"github.com/mblarsen/env-lease/internal/lease"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestApply(t *testing.T) {
	t.Run("returns raw secret for lease without transforms", func(t *testing.T) {
		l := lease.Lease{Source: "mock", Destination: ".env", LeaseType: lease.TypeEnv, Variable: "API_KEY"}

		result, err := Apply(l, "secret")

		require.NoError(t, err)
		require.False(t, result.IsExploded())
		require.Len(t, result.Secrets, 1)
		assert.Equal(t, l, result.Secrets[0].Lease)
		assert.Equal(t, "secret", result.Secrets[0].Value)
	})

	t.Run("returns transformed simple secret", func(t *testing.T) {
		l := lease.Lease{
			Source:      "mock",
			Destination: ".env",
			LeaseType:   lease.TypeEnv,
			Variable:    "API_KEY",
			Transform:   []string{"json", "select 'api.key'"},
		}

		result, err := Apply(l, `{"api":{"key":"selected-secret"}}`)

		require.NoError(t, err)
		require.False(t, result.IsExploded())
		require.Len(t, result.Secrets, 1)
		assert.Equal(t, l, result.Secrets[0].Lease)
		assert.Equal(t, "selected-secret", result.Secrets[0].Value)
	})

	t.Run("returns parent and sorted exploded child secrets", func(t *testing.T) {
		l := lease.Lease{
			Source:      "mock-explode",
			Destination: "/project/.env",
			LeaseType:   lease.TypeEnv,
			Transform:   []string{"json", "explode"},
		}

		result, err := Apply(l, `{"B_KEY":"b","A_KEY":"a"}`)

		require.NoError(t, err)
		require.True(t, result.IsExploded())
		require.NotNil(t, result.Parent)
		assert.Equal(t, "", result.Parent.Variable)
		assert.Equal(t, "", result.Parent.ParentSource)
		require.Len(t, result.Secrets, 2)

		parentID := lease.ParentIdentity("mock-explode", "/project/.env")
		assert.Equal(t, "A_KEY", result.Secrets[0].Lease.Variable)
		assert.Equal(t, parentID, result.Secrets[0].Lease.ParentSource)
		assert.Equal(t, "a", result.Secrets[0].Value)
		assert.Equal(t, "B_KEY", result.Secrets[1].Lease.Variable)
		assert.Equal(t, parentID, result.Secrets[1].Lease.ParentSource)
		assert.Equal(t, "b", result.Secrets[1].Value)
	})

	t.Run("rejects explode for file leases", func(t *testing.T) {
		l := lease.Lease{Source: "mock-explode", Destination: "/project/secret", LeaseType: lease.TypeFile, Transform: []string{"json", "explode"}}

		_, err := Apply(l, `{"A_KEY":"a"}`)

		require.Error(t, err)
		assert.Contains(t, err.Error(), "'explode' transform cannot be used with lease_type 'file'")
	})

	t.Run("rejects non-terminal structured data", func(t *testing.T) {
		l := lease.Lease{Source: "mock-json", Destination: "/project/.env", LeaseType: lease.TypeEnv, Variable: "JSON", Transform: []string{"json"}}

		_, err := Apply(l, `{"A_KEY":"a"}`)

		require.Error(t, err)
		assert.Contains(t, err.Error(), "transform pipeline must produce a string or exploded data")
	})
}
