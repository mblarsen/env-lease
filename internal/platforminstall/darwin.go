//go:build darwin
// +build darwin

package platforminstall

import (
	"context"
	"fmt"
	"path/filepath"
)

const (
	launchdDir              = "Library/LaunchAgents"
	darwinDaemonServiceName = "com.user.env-lease.plist"
	darwinIdleServiceName   = "com.user.env-lease-idle.plist"
)

type launchdPlatform struct{}

func newPlatform() platform { return launchdPlatform{} }

func (launchdPlatform) InstallDaemon(ctx context.Context, m *Manager, opts DaemonOptions) (Result, error) {
	plist := File{Path: daemonPlistPath(m.paths.HomeDir), Mode: 0644, Body: renderDaemonPlist(m.paths)}
	if err := writeAtomic(plist); err != nil {
		return Result{}, err
	}
	if err := m.runner.Run(ctx, "launchctl", "load", plist.Path); err != nil {
		return Result{}, fmt.Errorf("failed to load launchd service: %w", err)
	}
	return Result{Message: "Successfully installed and started daemon service.", Files: []File{plist}}, nil
}

func (launchdPlatform) UninstallDaemon(ctx context.Context, m *Manager) (Result, error) {
	plistPath := daemonPlistPath(m.paths.HomeDir)
	_ = m.runner.Run(ctx, "launchctl", "unload", plistPath)
	_ = removeIgnoringMissing(plistPath)
	return Result{Message: "Successfully uninstalled daemon service."}, nil
}

func (launchdPlatform) StatusDaemon(ctx context.Context, m *Manager) (Status, error) {
	plistPath := daemonPlistPath(m.paths.HomeDir)
	installed, err := fileExists(plistPath)
	return Status{Installed: installed, File: plistPath}, err
}

func (launchdPlatform) ReloadDaemon(ctx context.Context, m *Manager) (Result, error) {
	plistPath := daemonPlistPath(m.paths.HomeDir)
	if err := m.runner.Run(ctx, "launchctl", "unload", plistPath); err != nil {
		return Result{}, fmt.Errorf("failed to unload launchd service: %w", err)
	}
	if err := m.runner.Run(ctx, "launchctl", "load", plistPath); err != nil {
		return Result{}, fmt.Errorf("failed to load launchd service: %w", err)
	}
	return Result{Message: "Successfully reloaded daemon service."}, nil
}

func (launchdPlatform) InstallIdle(ctx context.Context, m *Manager, opts IdleOptions) (Result, error) {
	script := idleScriptFile(m.paths.HomeDir)
	plist := File{Path: idlePlistPath(m.paths.HomeDir), Mode: 0644, Body: renderIdlePlist(m.paths, opts, script.Path)}
	for _, file := range []File{script, plist} {
		if err := writeAtomic(file); err != nil {
			return Result{}, err
		}
	}
	if err := m.runner.Run(ctx, "launchctl", "load", plist.Path); err != nil {
		return Result{}, fmt.Errorf("failed to load launchd service: %w", err)
	}
	return Result{Message: fmt.Sprintf("Successfully installed and started idle revocation service. Timeout: %s, Check Interval: %s", opts.TimeoutText, opts.CheckIntervalText), Files: []File{script, plist}}, nil
}

func (launchdPlatform) UninstallIdle(ctx context.Context, m *Manager) (Result, error) {
	plistPath := idlePlistPath(m.paths.HomeDir)
	scriptPath := idleScriptPath(m.paths.HomeDir)
	_ = m.runner.Run(ctx, "launchctl", "unload", plistPath)
	_ = removeIgnoringMissing(plistPath)
	_ = removeIgnoringMissing(scriptPath)
	return Result{Message: "Successfully uninstalled idle revocation service."}, nil
}

func (launchdPlatform) StatusIdle(ctx context.Context, m *Manager) (Status, error) {
	plistPath := idlePlistPath(m.paths.HomeDir)
	installed, err := fileExists(plistPath)
	return Status{Installed: installed, File: plistPath}, err
}

func daemonPlistPath(homeDir string) string {
	return filepath.Join(homeDir, launchdDir, darwinDaemonServiceName)
}

func idlePlistPath(homeDir string) string {
	return filepath.Join(homeDir, launchdDir, darwinIdleServiceName)
}

func idleScriptPath(homeDir string) string {
	return filepath.Join(homeDir, scriptDir, idleScriptName)
}

func idleScriptFile(homeDir string) File {
	return File{Path: idleScriptPath(homeDir), Mode: 0755, Body: idleRevokeScript}
}

func renderDaemonPlist(paths Paths) string {
	return fmt.Sprintf(daemonPlistTemplate, paths.Executable, paths.HomeDir, paths.HomeDir)
}

func renderIdlePlist(paths Paths, opts IdleOptions, scriptPath string) string {
	return fmt.Sprintf(idlePlistTemplate, scriptPath, int(opts.Timeout.Seconds()), paths.Executable, int(opts.CheckInterval.Seconds()), paths.HomeDir, paths.HomeDir)
}

const daemonPlistTemplate = `<?xml version="1.0" encoding="UTF-8"?>
<!DOCTYPE plist PUBLIC "-//Apple//DTD PLIST 1.0//EN" "http://www.apple.com/DTDs/PropertyList-1.0.dtd">
<plist version="1.0">
<dict>
    <key>Label</key>
    <string>com.user.env-lease</string>
    <key>ProgramArguments</key>
    <array>
        <string>%s</string>
        <string>daemon</string>
        <string>run</string>
    </array>
    <key>RunAtLoad</key>
    <true/>
    <key>KeepAlive</key>
    <true/>
    <key>StandardOutPath</key>
    <string>%s/Library/Logs/env-lease.log</string>
    <key>StandardErrorPath</key>
    <string>%s/Library/Logs/env-lease.error.log</string>
</dict>
</plist>
`

const idlePlistTemplate = `<?xml version="1.0" encoding="UTF-8"?>
<!DOCTYPE plist PUBLIC "-//Apple//DTD PLIST 1.0//EN" "http://www.apple.com/DTDs/PropertyList-1.0.dtd">
<plist version="1.0">
<dict>
    <key>Label</key>
    <string>com.user.env-lease-idle</string>
    <key>ProgramArguments</key>
    <array>
        <string>/bin/sh</string>
        <string>%s</string>
        <string>%d</string>
        <string>%s</string>
    </array>
    <key>RunAtLoad</key>
    <true/>
    <key>StartInterval</key>
    <integer>%d</integer> <!-- Run every X seconds -->
    <key>StandardOutPath</key>
    <string>%s/Library/Logs/env-lease-idle.log</string>
    <key>StandardErrorPath</key>
    <string>%s/Library/Logs/env-lease-idle.error.log</string>
</dict>
</plist>
`
