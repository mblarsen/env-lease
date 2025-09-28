# env-lease Usage Guide

`env-lease` is a command-line tool for managing temporary, leased secrets in local development environment files.

## Getting Started

### 1. Installation (macOS & Linux)

The recommended way to install `env-lease` is via Homebrew:

```sh
brew install mblarsen/tap/env-lease
```

### 2. Install and Start the Daemon

The daemon is a background service that manages the lease timers.

```sh
# Install and start the daemon as a user service
env-lease daemon install
```

### 3. Configure Your First Lease

Create a file named `env-lease.toml` in your project's root directory:

```toml
# env-lease.toml

[[lease]]
# The 1Password secret reference URI
source = "op://your_vault/your_item/your_field"
# The destination file for the secret
destination = ".envrc"
# The environment variable to set
variable = "API_KEY"
# The duration of the lease
duration = "1h"
```

### 4. Grant the Lease

Run the `grant` command to fetch the secret and start the lease:

```sh
env-lease grant
```

### 5. Check the Status

You can see all active leases and their remaining time with the `status` command:

```sh
env-lease status
```

---

## Configuration (`env-lease.toml`)

The `env-lease.toml` file is the heart of the configuration. It's a declarative file that defines all the leases for a project.

### Lease Options

| Key           | Required | Description                                                                                             | Example                               |
|---------------|----------|---------------------------------------------------------------------------------------------------------|---------------------------------------|
| `source`      | Yes      | The URI of the secret. For 1Password, this is the `op://` reference.                                   | `"op://vault/item/secret"`            |
| `destination` | Yes      | The relative path to the target file.                                                                   | `".envrc"`                            |
| `duration`    | Yes      | The lease duration (e.g., "10m", "1h", "8h").                                                           | `"8h"`                                |
| `lease_type`  | No       | The type of lease. Can be `"env"` (default) or `"file"`.                                                | `"file"`                              |
| `variable`    | Yes      | The name of the environment variable to set (for `lease_type="env"`).                                   | `"API_KEY"`                           |
| `format`      | No       | A Go `sprintf`-style format string for `env` leases. Defaults are applied for `.env` and `.envrc`.    | `"export %s=%q"`                      |
| `encoding`    | No       | If set to `"base64"`, the secret value is base64-encoded (for `lease_type="env"`).                        | `"base64"`                            |
| `op_account`  | No       | The 1Password account to use. Overrides the `OP_ACCOUNT` environment variable.                      | `"my-account"`                        |

---

## Command Reference

| Command                  | Description                                                                                               |
|--------------------------|-----------------------------------------------------------------------------------------------------------|
| `env-lease grant`        | Grants all leases defined in `env-lease.toml`. Flags: `--override`, `--continue-on-error`.                 |
| `env-lease revoke`       | Immediately revokes all secrets defined in the current project's `env-lease.toml`.                          |
| `env-lease status`       | Lists all currently active leases managed by the daemon.                                                  |
| `env-lease enable-notifications` | (macOS only) Guides the user to grant notification permissions.                                           |
| `env-lease daemon install`| Installs and starts the daemon as a user service.                                                         |
| `env-lease daemon uninstall`| Stops and uninstalls the daemon.                                                                          |
| `env-lease daemon cleanup`| Manually purges all orphaned leases from the daemon's state.                                              |

---

## 1Password Examples

### Example 1: Lease to an Environment Variable

This is the most common use case. The secret is leased to an environment variable in a file like `.envrc`.

```toml
# env-lease.toml
[[lease]]
source = "op://your_vault/api-key/credential"
destination = ".envrc"
variable = "API_KEY"
duration = "1h"
# The %q verb safely quotes the secret
format = "export %s=%q"
```

### Example 2: Lease to a File

This is useful for secrets that are files themselves, like a `.pem` key or a `google-service.json` file.

```toml
# env-lease.toml
[[lease]]
lease_type = "file"
source = "op://your_vault/gcp-key/document"
destination = "gcp-key.json"
duration = "8h"
```

When granted, this will create a `gcp-key.json` file in your project directory. When the lease expires, the file will be deleted.

---

## Security Model

`env-lease` is designed with a simple and secure IPC model for local development.

### Client-Daemon Architecture

The `env-lease` CLI communicates with a background `env-leased` daemon via a Unix Domain Socket. This socket is created in a user-specific directory with restrictive file permissions, meaning other users on the system cannot access it.

### HMAC Token Authentication

To protect against other processes running *as your user* from interfering with the daemon, `env-lease` uses a shared secret token and HMAC-SHA256 signatures for all communication.

*   **Mechanism:** On its first run, the daemon generates a cryptographically random token and stores it in `~/.config/env-lease/auth.token` with `0600` permissions. Every command sent from the CLI to the daemon is signed with this token. The daemon verifies the signature on every message it receives.
*   **Protection:** This ensures that the daemon only acts on legitimate, untampered commands from the official `env-lease` CLI, and prevents other processes from sending unauthorized or malicious commands.

### Limitations

The security model is designed to raise the bar for attack and prevent accidental interference. It does **not** protect against a sophisticated attacker who has already fully compromised your user account, as such an attacker could read the auth token itself.
