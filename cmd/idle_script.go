package cmd

const idleRevokeScript = `#!/bin/sh
# This script checks for user idle time and revokes all env-lease leases if the idle time exceeds a given threshold.

LOG_FILE="$HOME/Library/Logs/env-lease-idle-debug.log"
echo "---" >> "$LOG_FILE"
echo "Running idle check at $(date)" >> "$LOG_FILE"

# The timeout in seconds. This is the first argument to the script.
TIMEOUT_SECONDS=$1

# The path to the env-lease executable. This is the second argument.
ENV_LEASE_CMD=$2

get_idle_time_seconds() {
    os_name=$(uname)
    if [ "$os_name" = "Darwin" ]; then
        # On macOS, use ioreg to get idle time in nanoseconds and convert to seconds.
        idle_ns=$(ioreg -c IOHIDSystem | awk '/HIDIdleTime/ {print $NF; exit}')
        echo $((idle_ns / 1000000000))
    elif [ "$os_name" = "Linux" ]; then
        # On Linux, try xprintidle first (for X11).
        if command -v xprintidle >/dev/null 2>&1; then
            echo $(( $(xprintidle) / 1000 ))
        # Fallback to D-Bus for GNOME/Wayland.
        elif command -v dbus-send >/dev/null 2>&1; then
            dbus-send --print-reply --dest=org.gnome.Mutter.IdleMonitor /org/gnome/Mutter/IdleMonitor/Core org.gnome.Mutter.IdleMonitor.GetIdletime | awk 'END {print $2 / 1000}'
        else
            # If no method is available, return 0 to prevent accidental revocation.
            echo 0
        fi
    else
        # Unsupported OS, return 0.
        echo 0
    fi
}

IDLE_TIME=$(get_idle_time_seconds)

echo "System idle time: ${IDLE_TIME}s. Timeout is: ${TIMEOUT_SECONDS}s." >> "$LOG_FILE"

if [ "$IDLE_TIME" -gt "$TIMEOUT_SECONDS" ]; then
    echo "Idle time exceeded. Revoking all leases." >> "$LOG_FILE"
    "$ENV_LEASE_CMD" revoke --all >> "$LOG_FILE" 2>&1
else
    echo "Idle time is within the limit. No action taken." >> "$LOG_FILE"
fi
`
