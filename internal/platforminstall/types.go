package platforminstall

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"time"
)

// Runner executes platform service-manager commands.
type Runner interface {
	Run(ctx context.Context, name string, args ...string) error
	Output(ctx context.Context, name string, args ...string) ([]byte, error)
}

type execRunner struct{}

func (execRunner) Run(ctx context.Context, name string, args ...string) error {
	return exec.CommandContext(ctx, name, args...).Run()
}

func (execRunner) Output(ctx context.Context, name string, args ...string) ([]byte, error) {
	return exec.CommandContext(ctx, name, args...).Output()
}

// File contains an installation artifact rendered for the current platform.
type File struct {
	Path string
	Mode os.FileMode
	Body string
}

// Result describes the user-visible outcome of an installer operation.
type Result struct {
	Message string
	Files   []File
	Output  string
}

// Status describes whether an installer artifact is present.
type Status struct {
	Installed bool
	File      string
	Output    string
	Warning   string
}

// IdleOptions configures idle-based lease revocation installation.
type IdleOptions struct {
	Timeout           time.Duration
	CheckInterval     time.Duration
	TimeoutText       string
	CheckIntervalText string
}

// DaemonOptions configures daemon service installation.
type DaemonOptions struct {
	PrintOnly bool
}

// Paths contains the shared filesystem inputs used to render platform artifacts.
type Paths struct {
	Executable string
	HomeDir    string
}

// Manager installs daemon and idle platform artifacts for env-lease.
type Manager struct {
	platform platform
	runner   Runner
	paths    Paths
}

type platform interface {
	InstallDaemon(context.Context, *Manager, DaemonOptions) (Result, error)
	UninstallDaemon(context.Context, *Manager) (Result, error)
	StatusDaemon(context.Context, *Manager) (Status, error)
	ReloadDaemon(context.Context, *Manager) (Result, error)
	InstallIdle(context.Context, *Manager, IdleOptions) (Result, error)
	UninstallIdle(context.Context, *Manager) (Result, error)
	StatusIdle(context.Context, *Manager) (Status, error)
}

// NewManager constructs a Manager with explicit dependencies for tests and adapters.
func NewManager(platform platform, runner Runner, paths Paths) *Manager {
	if runner == nil {
		runner = execRunner{}
	}
	return &Manager{platform: platform, runner: runner, paths: paths}
}

// DefaultManager constructs a Manager for the current OS and executable.
func DefaultManager() (*Manager, error) {
	executable, err := os.Executable()
	if err != nil {
		return nil, err
	}
	homeDir, err := os.UserHomeDir()
	if err != nil {
		return nil, err
	}
	return NewManager(newPlatform(), execRunner{}, Paths{Executable: executable, HomeDir: homeDir}), nil
}

func (m *Manager) InstallDaemon(ctx context.Context, opts DaemonOptions) (Result, error) {
	return m.platform.InstallDaemon(ctx, m, opts)
}

func (m *Manager) UninstallDaemon(ctx context.Context) (Result, error) {
	return m.platform.UninstallDaemon(ctx, m)
}

func (m *Manager) StatusDaemon(ctx context.Context) (Status, error) {
	return m.platform.StatusDaemon(ctx, m)
}

func (m *Manager) ReloadDaemon(ctx context.Context) (Result, error) {
	return m.platform.ReloadDaemon(ctx, m)
}

func (m *Manager) InstallIdle(ctx context.Context, opts IdleOptions) (Result, error) {
	if opts.Timeout <= 0 {
		return Result{}, fmt.Errorf("idle timeout must be positive")
	}
	if opts.CheckInterval <= 0 {
		return Result{}, fmt.Errorf("idle check interval must be positive")
	}
	if opts.TimeoutText == "" {
		opts.TimeoutText = opts.Timeout.String()
	}
	if opts.CheckIntervalText == "" {
		opts.CheckIntervalText = opts.CheckInterval.String()
	}
	return m.platform.InstallIdle(ctx, m, opts)
}

func (m *Manager) UninstallIdle(ctx context.Context) (Result, error) {
	return m.platform.UninstallIdle(ctx, m)
}

func (m *Manager) StatusIdle(ctx context.Context) (Status, error) {
	return m.platform.StatusIdle(ctx, m)
}
