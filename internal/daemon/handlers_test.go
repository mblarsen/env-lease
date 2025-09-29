package daemon

import (
	"encoding/json"
	"testing"
	"time"

	"github.com/mblarsen/env-lease/internal/ipc"
)

func TestHandleGrant(t *testing.T) {
	state := NewState()
	clock := &mockClock{now: time.Now()}
	revoker := &mockRevoker{}
	notifier := &mockNotifier{}
	daemon := NewDaemon(state, "/dev/null", clock, nil, revoker, notifier)

	lease := ipc.Lease{
		Source:      "1password",
		Destination: "/tmp/foo",
		LeaseType:   "env",
		Variable:    "MY_VAR",
		Duration:    "1h",
	}
	req := ipc.GrantRequest{
		Command:  "grant",
		Leases:   []ipc.Lease{lease},
	}
	payload, _ := json.Marshal(req)

	_, err := daemon.handleGrant(payload)
	if err != nil {
		t.Fatalf("handleGrant failed: %v", err)
	}

	key := "1password;/tmp/foo;MY_VAR"
	if _, ok := daemon.state.Leases[key]; !ok {
		t.Fatal("lease not found in state")
	}

	expectedExpiresAt := clock.now.Add(time.Hour)
	if !daemon.state.Leases[key].ExpiresAt.Equal(expectedExpiresAt) {
		t.Fatalf("expected expiresAt %v, got %v", expectedExpiresAt, daemon.state.Leases[key].ExpiresAt)
	}
}

func TestHandleGrant_RevokesRemovedLeases(t *testing.T) {
	state := NewState()
	clock := &mockClock{now: time.Now()}
	revoker := &mockRevoker{}
	notifier := &mockNotifier{}
	daemon := NewDaemon(state, "/dev/null", clock, nil, revoker, notifier)

	// Grant two leases
	lease1 := ipc.Lease{
		Source:      "1password",
		Destination: "/tmp/foo",
		LeaseType:   "env",
		Variable:    "MY_VAR_1",
		Duration:    "1h",
	}
	lease2 := ipc.Lease{
		Source:      "1password",
		Destination: "/tmp/foo",
		LeaseType:   "env",
		Variable:    "MY_VAR_2",
		Duration:    "1h",
	}
	req := ipc.GrantRequest{
		Command:    "grant",
		Leases:     []ipc.Lease{lease1, lease2},
		ConfigFile: "/tmp/env-lease.toml",
	}
	payload, _ := json.Marshal(req)
	_, err := daemon.handleGrant(payload)
	if err != nil {
		t.Fatalf("handleGrant failed: %v", err)
	}

	// Grant only one lease
	req = ipc.GrantRequest{
		Command:    "grant",
		Leases:     []ipc.Lease{lease1},
		ConfigFile: "/tmp/env-lease.toml",
	}
	payload, _ = json.Marshal(req)
	_, err = daemon.handleGrant(payload)
	if err != nil {
		t.Fatalf("handleGrant failed: %v", err)
	}

	// Check that the second lease was revoked
	key1 := "1password;/tmp/foo;MY_VAR_1"
	key2 := "1password;/tmp/foo;MY_VAR_2"
	if _, ok := daemon.state.Leases[key1]; !ok {
		t.Fatal("lease 1 not found in state")
	}
	if _, ok := daemon.state.Leases[key2]; ok {
		t.Fatal("lease 2 should have been revoked")
	}
	if len(revoker.revoked) != 1 {
		t.Fatalf("expected 1 revoked lease, got %d", len(revoker.revoked))
	}
	if revoker.revoked[0].Variable != "MY_VAR_2" {
		t.Fatalf("expected revoked lease to be MY_VAR_2, got %s", revoker.revoked[0].Variable)
	}
}