package lease

import (
	"path/filepath"
	"testing"

	"github.com/mblarsen/env-lease/internal/config"
)

func TestNormalizeDefaultsAndCanonicalizes(t *testing.T) {
	root := t.TempDir()
	cfg := &config.Config{
		Root: root,
		Lease: []config.Lease{
			{
				Source:      "op://vault/item/secret",
				Destination: ".envrc",
				Duration:    "1h",
				Variable:    "API_KEY",
			},
		},
	}

	set, err := Normalize(cfg, filepath.Join(root, "env-lease.toml"))
	if err != nil {
		t.Fatalf("Normalize failed: %v", err)
	}

	l := set.Leases[0]
	if l.LeaseType != TypeEnv {
		t.Fatalf("expected default lease type %q, got %q", TypeEnv, l.LeaseType)
	}
	if l.Provider != "1password" {
		t.Fatalf("expected default provider 1password, got %q", l.Provider)
	}
	if l.Format != "export %s=%q" {
		t.Fatalf("expected .envrc default format, got %q", l.Format)
	}
	expectedDest := filepath.Join(root, ".envrc")
	if l.Destination != expectedDest {
		t.Fatalf("expected canonical destination %q, got %q", expectedDest, l.Destination)
	}
	if l.Identity() != "op://vault/item/secret;"+expectedDest+";API_KEY" {
		t.Fatalf("unexpected identity %q", l.Identity())
	}
}

func TestNormalizeFileLeaseDefaultDestination(t *testing.T) {
	root := t.TempDir()
	cfg := &config.Config{
		Root: root,
		Lease: []config.Lease{
			{
				Source:    "op+file://app-iac container env/container_env.json",
				LeaseType: TypeFile,
				Duration:  "1h",
			},
		},
	}

	set, err := Normalize(cfg, filepath.Join(root, "env-lease.toml"))
	if err != nil {
		t.Fatalf("Normalize failed: %v", err)
	}

	expectedDest := filepath.Join(root, "container_env.json")
	if set.Leases[0].Destination != expectedDest {
		t.Fatalf("expected destination %q, got %q", expectedDest, set.Leases[0].Destination)
	}
}

func TestNormalizeValidatesSemanticRequirements(t *testing.T) {
	root := t.TempDir()
	cfg := &config.Config{
		Root: root,
		Lease: []config.Lease{
			{
				Destination: ".envrc",
				Duration:    "1h",
			},
		},
	}

	_, err := Normalize(cfg, filepath.Join(root, "env-lease.toml"))
	if err == nil {
		t.Fatal("expected missing source error")
	}
}

func TestNormalizePartialKeepsValidLeases(t *testing.T) {
	root := t.TempDir()
	cfg := &config.Config{
		Root: root,
		Lease: []config.Lease{
			{
				Source:      "op://vault/item/secret",
				Destination: ".env",
				Duration:    "1h",
				Variable:    "API_KEY",
			},
			{
				Destination: ".env",
				Duration:    "1h",
				Variable:    "MISSING_SOURCE",
			},
		},
	}

	set, errs := NormalizePartial(cfg, filepath.Join(root, "env-lease.toml"))
	if len(errs) != 1 {
		t.Fatalf("expected one invalid lease error, got %d", len(errs))
	}
	if len(set.Leases) != 1 {
		t.Fatalf("expected one valid lease, got %d", len(set.Leases))
	}
	if set.Leases[0].Variable != "API_KEY" {
		t.Fatalf("expected valid lease to remain, got %q", set.Leases[0].Variable)
	}
}

func TestNormalizeExplodeParentIdentity(t *testing.T) {
	root := t.TempDir()
	cfg := &config.Config{
		Root: root,
		Lease: []config.Lease{
			{
				Source:      "op://vault/item/env",
				Destination: ".env",
				Duration:    "1h",
				Transform:   []string{"json", "explode"},
			},
		},
	}

	set, err := Normalize(cfg, filepath.Join(root, "env-lease.toml"))
	if err != nil {
		t.Fatalf("Normalize failed: %v", err)
	}

	l := set.Leases[0]
	if !l.IsExplode() {
		t.Fatal("expected lease to be explode")
	}
	expectedParent := ParentIdentity(l.Source, l.Destination)
	if l.ParentIdentity() != expectedParent {
		t.Fatalf("expected parent identity %q, got %q", expectedParent, l.ParentIdentity())
	}
}
