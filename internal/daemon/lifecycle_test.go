package daemon

import (
	"testing"
	"time"

	"github.com/mblarsen/env-lease/internal/ipc"
	"github.com/mblarsen/env-lease/internal/lease"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestLifecycleGrantReconcilesRemovedLeases(t *testing.T) {
	state := NewState()
	clock := &mockClock{now: time.Now()}
	revoker := &mockRevoker{}
	lifecycle := NewLifecycle(state, "/dev/null", clock, revoker, nil)

	lease1 := ipc.Lease{
		Source:      "1password",
		Destination: "/tmp/foo",
		LeaseType:   lease.TypeEnv,
		Variable:    "MY_VAR_1",
		Duration:    "1h",
	}
	lease2 := ipc.Lease{
		Source:      "1password",
		Destination: "/tmp/foo",
		LeaseType:   lease.TypeEnv,
		Variable:    "MY_VAR_2",
		Duration:    "1h",
	}

	count, err := lifecycle.Grant(ipc.GrantRequest{
		Leases:     []ipc.Lease{lease1, lease2},
		ConfigFile: "/tmp/env-lease.toml",
	})
	require.NoError(t, err)
	assert.Equal(t, 2, count)

	count, err = lifecycle.Grant(ipc.GrantRequest{
		Leases:     []ipc.Lease{lease1},
		ConfigFile: "/tmp/env-lease.toml",
	})
	require.NoError(t, err)
	assert.Equal(t, 1, count)

	assert.Contains(t, state.Leases, "1password;/tmp/foo;MY_VAR_1")
	assert.NotContains(t, state.Leases, "1password;/tmp/foo;MY_VAR_2")
	require.Len(t, revoker.revoked, 1)
	assert.Equal(t, "MY_VAR_2", revoker.revoked[0].Variable)
}

func TestLifecycleRevokeAllReturnsShellCommands(t *testing.T) {
	state := NewState()
	state.Leases["env"] = &lease.Lease{
		Source:      "1password://vault/item/env",
		Destination: "/tmp/env",
		LeaseType:   lease.TypeEnv,
		Variable:    "ENV_VAR",
	}
	state.Leases["shell"] = &lease.Lease{
		Source:    "1password://vault/item/shell",
		LeaseType: lease.TypeShell,
		Variable:  "SHELL_VAR",
	}

	revoker := &mockRevoker{}
	lifecycle := NewLifecycle(state, "/dev/null", &mockClock{now: time.Now()}, revoker, &mockNotifier{})

	result, err := lifecycle.Revoke(ipc.RevokeRequest{All: true})
	require.NoError(t, err)

	assert.Equal(t, 2, result.Count)
	assert.Equal(t, []string{"unset SHELL_VAR"}, result.ShellCommands)
	assert.Equal(t, 2, revoker.RevokeCount)
	assert.Empty(t, state.Leases)
}
