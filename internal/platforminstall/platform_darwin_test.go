//go:build darwin
// +build darwin

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

func TestLaunchdDaemonInstallWritesPlistAndLoadsIt(t *testing.T) {
	home := t.TempDir()
	runner := &recordingRunner{}
	manager := NewManager(newPlatform(), runner, Paths{Executable: "/usr/local/bin/env-lease", HomeDir: home})

	result, err := manager.InstallDaemon(context.Background(), DaemonOptions{})

	require.NoError(t, err)
	plistPath := filepath.Join(home, launchdDir, darwinDaemonServiceName)
	assert.Equal(t, "Successfully installed and started daemon service.", result.Message)
	assert.Equal(t, []string{"launchctl load " + plistPath}, runner.commands)
	body, err := os.ReadFile(plistPath)
	require.NoError(t, err)
	assert.Contains(t, string(body), "<string>/usr/local/bin/env-lease</string>")
	assert.Contains(t, string(body), home+"/Library/Logs/env-lease.log")
}

func TestLaunchdIdleInstallWritesScriptAndPlist(t *testing.T) {
	home := t.TempDir()
	runner := &recordingRunner{}
	manager := NewManager(newPlatform(), runner, Paths{Executable: "/bin/env-lease", HomeDir: home})

	_, err := manager.InstallIdle(context.Background(), IdleOptions{Timeout: time.Hour, CheckInterval: 5 * time.Minute})

	require.NoError(t, err)
	scriptPath := filepath.Join(home, scriptDir, idleScriptName)
	plistPath := filepath.Join(home, launchdDir, darwinIdleServiceName)
	assert.Equal(t, []string{"launchctl load " + plistPath}, runner.commands)
	assert.FileExists(t, scriptPath)
	body, err := os.ReadFile(plistPath)
	require.NoError(t, err)
	assert.Contains(t, string(body), "<string>"+scriptPath+"</string>")
	assert.Contains(t, string(body), "<string>3600</string>")
	assert.Contains(t, string(body), "<integer>300</integer>")
}

func TestLaunchdReloadPlansUnloadThenLoad(t *testing.T) {
	home := t.TempDir()
	runner := &recordingRunner{}
	manager := NewManager(newPlatform(), runner, Paths{Executable: "/bin/env-lease", HomeDir: home})

	_, err := manager.ReloadDaemon(context.Background())

	require.NoError(t, err)
	plistPath := filepath.Join(home, launchdDir, darwinDaemonServiceName)
	assert.Equal(t, []string{"launchctl unload " + plistPath, "launchctl load " + plistPath}, runner.commands)
}

func TestIdleOptionsRejectNonPositiveDurations(t *testing.T) {
	manager := NewManager(newPlatform(), &recordingRunner{}, Paths{Executable: "/bin/env-lease", HomeDir: t.TempDir()})

	_, err := manager.InstallIdle(context.Background(), IdleOptions{Timeout: 0, CheckInterval: time.Minute})

	require.Error(t, err)
	assert.Contains(t, err.Error(), "idle timeout must be positive")
}

func TestLaunchdStatusReportsArtifactPresence(t *testing.T) {
	home := t.TempDir()
	manager := NewManager(newPlatform(), &recordingRunner{}, Paths{Executable: "/bin/env-lease", HomeDir: home})

	status, err := manager.StatusDaemon(context.Background())
	require.NoError(t, err)
	assert.False(t, status.Installed)

	require.NoError(t, os.MkdirAll(filepath.Dir(daemonPlistPath(home)), 0755))
	require.NoError(t, os.WriteFile(daemonPlistPath(home), []byte("plist"), 0644))
	status, err = manager.StatusDaemon(context.Background())
	require.NoError(t, err)
	assert.True(t, status.Installed)
	assert.Equal(t, daemonPlistPath(home), status.File)
}

func TestRecordingRunnerCanReturnOutputErrors(t *testing.T) {
	runner := &recordingRunner{outErr: errors.New("boom")}
	_, err := runner.Output(context.Background(), "cmd")
	assert.Error(t, err)
}
