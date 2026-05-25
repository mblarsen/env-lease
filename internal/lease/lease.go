// Package lease owns the semantic runtime model for env-lease leases.
package lease

import (
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/mblarsen/env-lease/internal/config"
	"github.com/mblarsen/env-lease/internal/fileutil"
	"github.com/mblarsen/env-lease/internal/ipc"
)

const (
	// TypeEnv is a lease written as an environment variable in a destination file.
	TypeEnv = "env"
	// TypeFile is a lease written as a whole file.
	TypeFile = "file"
	// TypeShell is a lease emitted as shell commands instead of written to disk.
	TypeShell = "shell"
)

// Set is the normalized runtime shape for one env-lease configuration file.
type Set struct {
	Root       string
	ConfigFile string
	Leases     []Lease
}

// Lease is the normalized runtime shape used by grant and daemon logic.
type Lease struct {
	Provider       string     `json:"provider,omitempty"`
	Source         string     `json:"source"`
	RawDestination string     `json:"raw_destination,omitempty"`
	Destination    string     `json:"destination"`
	Duration       string     `json:"duration"`
	LeaseType      string     `json:"lease_type"`
	Variable       string     `json:"variable,omitempty"`
	Format         string     `json:"format,omitempty"`
	Transform      []string   `json:"transform,omitempty"`
	FileMode       string     `json:"file_mode,omitempty"`
	OpAccount      string     `json:"op_account,omitempty"`
	ExpiresAt      time.Time  `json:"expires_at"`
	OrphanedSince  *time.Time `json:"orphaned_since,omitempty"`
	ConfigFile     string     `json:"config_file,omitempty"`
	ParentSource   string     `json:"parent_source,omitempty"`
}

// Normalize converts raw TOML config leases into the runtime Lease model.
func Normalize(cfg *config.Config, configFile string) (*Set, error) {
	set, errs := NormalizePartial(cfg, configFile)
	if len(errs) > 0 {
		return nil, errs[0]
	}
	return set, nil
}

// NormalizePartial converts all semantically valid raw TOML leases into the
// runtime Lease model and returns per-lease errors for invalid leases.
func NormalizePartial(cfg *config.Config, configFile string) (*Set, []error) {
	if cfg == nil {
		return nil, []error{fmt.Errorf("config is nil")}
	}

	absConfigFile := configFile
	if absConfigFile == "" {
		absConfigFile = filepath.Join(cfg.Root, "env-lease.toml")
	}
	if !filepath.IsAbs(absConfigFile) {
		abs, err := filepath.Abs(absConfigFile)
		if err != nil {
			return nil, []error{fmt.Errorf("could not get absolute path for config: %w", err)}
		}
		absConfigFile = abs
	}

	set := &Set{
		Root:       cfg.Root,
		ConfigFile: absConfigFile,
		Leases:     make([]Lease, 0, len(cfg.Lease)),
	}
	var errs []error

	for i, raw := range cfg.Lease {
		l, err := normalizeOne(cfg.Root, absConfigFile, raw)
		if err != nil {
			errs = append(errs, fmt.Errorf("lease %d: %w", i, err))
			continue
		}
		set.Leases = append(set.Leases, l)
	}

	return set, errs
}

func normalizeOne(root, configFile string, raw config.Lease) (Lease, error) {
	l := Lease{
		Provider:       raw.Provider,
		Source:         raw.Source,
		RawDestination: raw.Destination,
		Destination:    raw.Destination,
		Duration:       raw.Duration,
		LeaseType:      raw.LeaseType,
		Variable:       raw.Variable,
		Format:         raw.Format,
		Transform:      append([]string(nil), raw.Transform...),
		FileMode:       raw.FileMode,
		OpAccount:      raw.OpAccount,
		ExpiresAt:      raw.ExpiresAt,
		OrphanedSince:  raw.OrphanedSince,
		ConfigFile:     configFile,
		ParentSource:   raw.ParentSource,
	}

	if l.Source == "" {
		return Lease{}, fmt.Errorf("source is required")
	}
	if l.Duration == "" {
		return Lease{}, fmt.Errorf("duration is required")
	}
	if _, err := time.ParseDuration(l.Duration); err != nil {
		return Lease{}, fmt.Errorf("invalid duration %q: %w", l.Duration, err)
	}

	if l.LeaseType == "" {
		l.LeaseType = TypeEnv
	}
	if l.Provider == "" {
		l.Provider = "1password"
	}

	if len(l.Transform) == 0 && l.Format == "base64" {
		l.Transform = []string{"base64-encode"}
	}

	isExplode := l.IsExplode()
	switch l.LeaseType {
	case TypeEnv:
		if l.Destination == "" {
			return Lease{}, fmt.Errorf("destination is required for lease_type %q", l.LeaseType)
		}
		if l.Variable == "" && !isExplode {
			return Lease{}, fmt.Errorf("variable is required for lease_type %q", l.LeaseType)
		}
		if l.Format == "" {
			if format, err := defaultEnvFormat(l.Destination); err == nil {
				l.Format = format
			}
		}
	case TypeFile:
		if l.Destination == "" {
			l.Destination = filepath.Base(l.Source)
			l.RawDestination = l.Destination
		}
	case TypeShell:
		if l.Variable == "" && !isExplode {
			return Lease{}, fmt.Errorf("variable is required for lease_type %q", l.LeaseType)
		}
	default:
		return Lease{}, fmt.Errorf("unknown lease_type %q", l.LeaseType)
	}

	destination, err := canonicalDestination(root, l)
	if err != nil {
		return Lease{}, err
	}
	l.Destination = destination

	return l, nil
}

// WithDefaultFormat returns a copy with the default env format applied when it
// can be inferred from the destination. It errors only when an env lease needs a
// format and the destination does not imply one.
func (l Lease) WithDefaultFormat() (Lease, error) {
	if l.LeaseType != TypeEnv || l.Format != "" {
		return l, nil
	}
	format, err := defaultEnvFormat(l.Destination)
	if err != nil {
		return Lease{}, err
	}
	l.Format = format
	return l, nil
}

func defaultEnvFormat(destination string) (string, error) {
	switch filepath.Base(destination) {
	case ".envrc":
		return "export %s=%q", nil
	case ".env":
		return "%s=%q", nil
	default:
		return "", fmt.Errorf("lease for '%s' has no format specified", destination)
	}
}

func canonicalDestination(root string, l Lease) (string, error) {
	if l.LeaseType == TypeShell {
		return filepath.Join(root, "<shell>"), nil
	}

	destination, err := fileutil.ExpandPath(l.Destination)
	if err != nil {
		return "", fmt.Errorf("could not expand destination path: %w", err)
	}
	if !filepath.IsAbs(destination) {
		return filepath.Join(root, destination), nil
	}
	return filepath.Clean(destination), nil
}

// IsExplode reports whether the lease has an explode transformation step.
func (l Lease) IsExplode() bool {
	for _, t := range l.Transform {
		if strings.HasPrefix(strings.TrimSpace(t), "explode") {
			return true
		}
	}
	return false
}

// Identity returns the stable identity for a concrete lease.
func (l Lease) Identity() string {
	return Identity(l.Source, l.Destination, l.Variable)
}

// ParentIdentity returns the stable identity used by exploded child leases.
func (l Lease) ParentIdentity() string {
	return ParentIdentity(l.Source, l.Destination)
}

// Identity builds the stable identity for a concrete lease.
func Identity(source, destination, variable string) string {
	return source + ";" + destination + ";" + variable
}

// ParentIdentity builds the stable identity used by exploded child leases.
func ParentIdentity(source, destination string) string {
	return source + "->" + destination
}

// ToIPC converts a normalized lease to the IPC wire shape.
func (l Lease) ToIPC() ipc.Lease {
	return ipc.Lease{
		Source:       l.Source,
		Destination:  l.Destination,
		Duration:     l.Duration,
		LeaseType:    l.LeaseType,
		Variable:     l.Variable,
		Format:       l.Format,
		Transform:    append([]string(nil), l.Transform...),
		FileMode:     l.FileMode,
		ExpiresAt:    l.ExpiresAt,
		ConfigFile:   l.ConfigFile,
		OpAccount:    l.OpAccount,
		ParentSource: l.ParentSource,
	}
}

// FromIPC converts an IPC lease into the runtime lease shape. IPC leases are
// expected to already carry canonical destinations.
func FromIPC(l ipc.Lease) Lease {
	return Lease{
		Source:       l.Source,
		Destination:  l.Destination,
		Duration:     l.Duration,
		LeaseType:    l.LeaseType,
		Variable:     l.Variable,
		Format:       l.Format,
		Transform:    append([]string(nil), l.Transform...),
		FileMode:     l.FileMode,
		ExpiresAt:    l.ExpiresAt,
		ConfigFile:   l.ConfigFile,
		OpAccount:    l.OpAccount,
		ParentSource: l.ParentSource,
	}
}

// ParseFileMode parses the lease file mode, applying the provided default when unset.
func (l Lease) ParseFileMode(defaultMode os.FileMode) (os.FileMode, error) {
	return parseFileMode(l.FileMode, defaultMode)
}

func parseFileMode(fileModeStr string, defaultMode os.FileMode) (os.FileMode, error) {
	if fileModeStr == "" {
		return defaultMode, nil
	}
	mode, err := strconv.ParseUint(fileModeStr, 8, 32)
	if err != nil {
		return 0, fmt.Errorf("invalid file mode: %w", err)
	}
	return os.FileMode(mode), nil
}
