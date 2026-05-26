//go:build linux
// +build linux

package platforminstall

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
)

const (
	linuxDaemonServiceName = "env-lease.service"
	linuxIdleServiceName   = "env-lease-idle.service"
	linuxIdleTimerName     = "env-lease-idle.timer"
	systemdDir             = ".config/systemd/user"
)

type systemdPlatform struct{}

func newPlatform() platform { return systemdPlatform{} }

func (systemdPlatform) InstallDaemon(ctx context.Context, m *Manager, opts DaemonOptions) (Result, error) {
	service := File{Path: daemonServicePath(m.paths.HomeDir), Mode: 0644, Body: renderDaemonService(m.paths)}
	if opts.PrintOnly {
		return Result{Files: []File{service}, Output: service.Body, Message: "WARNING: Service configuration printed but not installed."}, nil
	}
	if err := writeAtomic(service); err != nil {
		return Result{}, err
	}
	if err := m.runner.Run(ctx, "systemctl", "--user", "enable", "--now", linuxDaemonServiceName); err != nil {
		return Result{}, err
	}
	return Result{Message: fmt.Sprintf("Successfully installed env-lease daemon service. Configuration file created at: %s", service.Path), Files: []File{service}}, nil
}

func (systemdPlatform) UninstallDaemon(ctx context.Context, m *Manager) (Result, error) {
	servicePath := daemonServicePath(m.paths.HomeDir)
	_ = m.runner.Run(ctx, "systemctl", "--user", "disable", "--now", linuxDaemonServiceName)
	if err := os.Remove(servicePath); err != nil {
		return Result{}, err
	}
	return Result{Message: "Successfully uninstalled env-lease daemon service."}, nil
}

func (systemdPlatform) StatusDaemon(ctx context.Context, m *Manager) (Status, error) {
	servicePath := daemonServicePath(m.paths.HomeDir)
	installed, err := fileExists(servicePath)
	return Status{Installed: installed, File: servicePath}, err
}

func (systemdPlatform) ReloadDaemon(ctx context.Context, m *Manager) (Result, error) {
	if err := m.runner.Run(ctx, "systemctl", "--user", "reload", linuxDaemonServiceName); err != nil {
		return Result{}, fmt.Errorf("failed to reload daemon service: %w", err)
	}
	return Result{Message: "Successfully reloaded env-lease daemon service."}, nil
}

func (systemdPlatform) InstallIdle(ctx context.Context, m *Manager, opts IdleOptions) (Result, error) {
	script := idleScriptFile(m.paths.HomeDir)
	service := File{Path: idleServicePath(m.paths.HomeDir), Mode: 0644, Body: renderIdleService(m.paths, opts, script.Path)}
	timer := File{Path: idleTimerPath(m.paths.HomeDir), Mode: 0644, Body: renderIdleTimer(opts)}
	for _, file := range []File{script, service, timer} {
		if err := writeAtomic(file); err != nil {
			return Result{}, err
		}
	}
	if err := m.runner.Run(ctx, "systemctl", "--user", "daemon-reload"); err != nil {
		return Result{}, fmt.Errorf("failed to reload systemd: %w", err)
	}
	if err := m.runner.Run(ctx, "systemctl", "--user", "enable", "--now", linuxIdleTimerName); err != nil {
		return Result{}, fmt.Errorf("failed to enable and start systemd timer: %w", err)
	}
	return Result{Message: fmt.Sprintf("Successfully installed and started idle revocation service. Timeout: %s, Check Interval: %s", opts.TimeoutText, opts.CheckIntervalText), Files: []File{script, service, timer}}, nil
}

func (systemdPlatform) UninstallIdle(ctx context.Context, m *Manager) (Result, error) {
	_ = m.runner.Run(ctx, "systemctl", "--user", "disable", "--now", linuxIdleTimerName)
	_ = removeIgnoringMissing(idleServicePath(m.paths.HomeDir))
	_ = removeIgnoringMissing(idleTimerPath(m.paths.HomeDir))
	_ = removeIgnoringMissing(idleScriptPath(m.paths.HomeDir))
	_ = m.runner.Run(ctx, "systemctl", "--user", "daemon-reload")
	return Result{Message: "Successfully uninstalled idle revocation service."}, nil
}

func (systemdPlatform) StatusIdle(ctx context.Context, m *Manager) (Status, error) {
	timerPath := idleTimerPath(m.paths.HomeDir)
	installed, err := fileExists(timerPath)
	if err != nil || !installed {
		return Status{Installed: installed, File: timerPath}, err
	}
	out, outputErr := m.runner.Output(ctx, "systemctl", "--user", "status", linuxIdleTimerName)
	status := Status{Installed: true, File: timerPath, Output: string(out)}
	if outputErr != nil {
		status.Warning = "Idle revocation service is installed but may not be running."
	}
	return status, nil
}

func daemonServicePath(homeDir string) string {
	return filepath.Join(homeDir, systemdDir, linuxDaemonServiceName)
}

func idleServicePath(homeDir string) string {
	return filepath.Join(homeDir, systemdDir, linuxIdleServiceName)
}

func idleTimerPath(homeDir string) string {
	return filepath.Join(homeDir, systemdDir, linuxIdleTimerName)
}

func idleScriptPath(homeDir string) string {
	return filepath.Join(homeDir, scriptDir, idleScriptName)
}

func idleScriptFile(homeDir string) File {
	return File{Path: idleScriptPath(homeDir), Mode: 0755, Body: idleRevokeScript}
}

func renderDaemonService(paths Paths) string {
	return fmt.Sprintf(daemonServiceTemplate, paths.Executable)
}

func renderIdleService(paths Paths, opts IdleOptions, scriptPath string) string {
	return fmt.Sprintf(idleServiceTemplate, scriptPath, int(opts.Timeout.Seconds()), paths.Executable)
}

func renderIdleTimer(opts IdleOptions) string {
	return fmt.Sprintf(timerTemplate, opts.CheckIntervalText, opts.CheckIntervalText)
}

const daemonServiceTemplate = `[Unit]
Description=env-lease daemon

[Service]
ExecStart=%s daemon run
Restart=always
Environment="ENV_LEASE_LOG_LEVEL=info"

[Install]
WantedBy=default.target
`

const idleServiceTemplate = `[Unit]
Description=env-lease idle lease revoker

[Service]
Type=oneshot
ExecStart=/bin/sh %s %d %s
`

const timerTemplate = `[Unit]
Description=Run env-lease idle revoker periodically

[Timer]
OnBootSec=%s
OnUnitActiveSec=%s
Unit=env-lease-idle.service

[Install]
WantedBy=timers.target
`
