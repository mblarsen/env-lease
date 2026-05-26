package presentation

import (
	"bytes"
	"strings"
	"testing"
	"time"
)

func TestTerminalRenderStatusPreservesTableAndHierarchy(t *testing.T) {
	t.Setenv("NO_COLOR", "1")
	now := time.Now()
	leases := []StatusLease{
		{Variable: "CHILD_B", Source: "op://vault/item", Destination: "/tmp/.env", ParentID: "parent", ExpiresAt: now.Add(time.Minute)},
		{ID: "parent", Variable: "", Source: "op://vault/item", Destination: "/tmp/.env", ExpiresAt: now.Add(time.Minute), LeaseType: "explode"},
		{Variable: "CHILD_A", Source: "op://vault/item", Destination: "/tmp/.env", ParentID: "parent", ExpiresAt: now.Add(time.Minute)},
	}

	var out bytes.Buffer
	NewTerminal().RenderStatus(leases, &out)
	text := out.String()

	if !strings.Contains(text, "VARIABLE") || !strings.Contains(text, "<exploded>") {
		t.Fatalf("missing status table labels: %q", text)
	}
	if !strings.Contains(text, " ├─ CHILD_A") || !strings.Contains(text, " └─ CHILD_B") {
		t.Fatalf("expected sorted child hierarchy, got %q", text)
	}
}

func TestTerminalMessagesPreserveUserFacingText(t *testing.T) {
	var out bytes.Buffer
	terminal := NewTerminal()

	terminal.Print(&out, MessageGrantShellHint)
	terminal.Print(&out, MessageRevokeSent)
	terminal.Print(&out, MessageDaemonOffline)

	text := out.String()
	for _, want := range []string{
		"# When using shell lease types run this command like `eval $(env-lease grant)`",
		"Revoke request sent.",
		"Error: env-lease daemon is not running. Please start it with 'env-lease daemon install'.",
	} {
		if !strings.Contains(text, want) {
			t.Fatalf("expected %q in %q", want, text)
		}
	}
}
