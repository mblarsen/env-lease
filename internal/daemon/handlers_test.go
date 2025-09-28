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