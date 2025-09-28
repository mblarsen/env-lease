package daemon

import (
	"context"
	"github.com/mblarsen/env-lease/internal/ipc"
	"testing"
	"time"
)

type mockClock struct {
	now time.Time
}

func (m *mockClock) Now() time.Time {
	return m.now
}

func (m *mockClock) Ticker(d time.Duration) *time.Ticker {
	return time.NewTicker(24 * time.Hour)
}

func (m *mockClock) Advance(d time.Duration) {
	m.now = m.now.Add(d)
}

// TODO: This is a placeholder test. A real test would need to capture stdout.
func TestDaemon_Run(t *testing.T) {
	state := NewState()
	clock := &mockClock{now: time.Now()}
	revoker := &mockRevoker{}
	server, err := ipc.NewServer("/tmp/env-lease-test.sock", []byte("secret"))
	if err != nil {
		t.Fatal(err)
	}
	daemon := NewDaemon(state, "/dev/null", clock, server, revoker)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	go func() {
		time.Sleep(100 * time.Millisecond)
		cancel()
	}()

	err = daemon.Run(ctx)
	if err != nil && err != context.Canceled {
		t.Errorf("unexpected error: %v", err)
	}
}
