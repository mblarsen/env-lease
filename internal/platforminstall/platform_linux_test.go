//go:build linux
// +build linux

package platforminstall

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type recordingRunner struct {
	commands []string
	output   []byte
	outErr   error
}

func (r *recordingRunner) Run(ctx context.Context, name string, args ...string) error {
	r.commands = append(r.commands, commandString(name, args...))
	return nil
}

func (r *recordingRunner) Output(ctx context.Context, name string, args ...string) ([]byte, error) {
	r.commands = append(r.commands, commandString(name, args...))
	return r.output, r.outErr
}

func commandString(name string, args ...string) string {
	cmd := name
	for _, arg := range args {
		cmd += " " + arg
	}
	return cmd
}

func TestSystemdDaemonInstallWritesServiceAndEnablesIt(t *testing.T) {
	home := t.TempDir()
	runner := &recordingRunner{}
	manager := NewManager(newPlatform(), runner, Paths{Executable: "/usr/local/bin/env-lease", HomeDir: home})

	result, err := manager.InstallDaemon(context.Background(), DaemonOptions{})

	require.NoError(t, err)
	servicePath := filepath.Join(home, systemdDir, linuxDaemonServiceName)
	assert.Equal(t, "Successfully installed env-lease daemon service. Configuration file created at: "+servicePath, result.Message)
	assert.Equal(t, []string{"systemctl --user enable --now env-lease.service"}, runner.commands)
	body, err := os.ReadFile(servicePath)
	require.NoError(t, err)
	assert.Contains(t, string(body), "ExecStart=/usr/local/bin/env-lease daemon run")
}

func TestSystemdDaemonPrintOnlyDoesNotWriteOrRun(t *testing.T) {
	home := t.TempDir()
	runner := &recordingRunner{}
	manager := NewManager(newPlatform(), runner, Paths{Executable: "/bin/env-lease", HomeDir: home})

	result, err := manager.InstallDaemon(context.Background(), DaemonOptions{PrintOnly: true})

	require.NoError(t, err)
	assert.Empty(t, runner.commands)
	assert.Contains(t, result.Output, "ExecStart=/bin/env-lease daemon run")
	assert.NoFileExists(t, daemonServicePath(home))
}

func TestSystemdIdleInstallWritesArtifactsAndEnablesTimer(t *testing.T) {
	home := t.TempDir()
	runner := &recordingRunner{}
	manager := NewManager(newPlatform(), runner, Paths{Executable: "/bin/env-lease", HomeDir: home})

	_, err := manager.InstallIdle(context.Background(), IdleOptions{Timeout: time.Hour, CheckInterval: 5 * time.Minute})

	require.NoError(t, err)
	scriptPath := filepath.Join(home, scriptDir, idleScriptName)
	servicePath := filepath.Join(home, systemdDir, linuxIdleServiceName)
	timerPath := filepath.Join(home, systemdDir, linuxIdleTimerName)
	assert.Equal(t, []string{"systemctl --user daemon-reload", "systemctl --user enable --now env-lease-idle.timer"}, runner.commands)
	assert.FileExists(t, scriptPath)
	service, err := os.ReadFile(servicePath)
	require.NoError(t, err)
	assert.Contains(t, string(service), "ExecStart=/bin/sh "+scriptPath+" 3600 /bin/env-lease")
	timer, err := os.ReadFile(timerPath)
	require.NoError(t, err)
	assert.Contains(t, string(timer), "OnBootSec=5m")
	assert.Contains(t, string(timer), "OnUnitActiveSec=5m")
}

func TestSystemdIdleStatusIncludesOutputWarningWhenStatusFails(t *testing.T) {
	home := t.TempDir()
	runner := &recordingRunner{outErr: errors.New("inactive")}
	manager := NewManager(newPlatform(), runner, Paths{Executable: "/bin/env-lease", HomeDir: home})
	require.NoError(t, os.MkdirAll(filepath.Dir(idleTimerPath(home)), 0755))
	require.NoError(t, os.WriteFile(idleTimerPath(home), []byte("timer"), 0644))

	status, err := manager.StatusIdle(context.Background())

	require.NoError(t, err)
	assert.True(t, status.Installed)
	assert.Equal(t, "Idle revocation service is installed but may not be running.", status.Warning)
	assert.Equal(t, []string{"systemctl --user status env-lease-idle.timer"}, runner.commands)
}

func TestIdleOptionsRejectNonPositiveDurations(t *testing.T) {
	manager := NewManager(newPlatform(), &recordingRunner{}, Paths{Executable: "/bin/env-lease", HomeDir: t.TempDir()})

	_, err := manager.InstallIdle(context.Background(), IdleOptions{Timeout: 0, CheckInterval: time.Minute})

	require.Error(t, err)
	assert.Contains(t, err.Error(), "idle timeout must be positive")
}
