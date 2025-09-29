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
| `source`      | Yes      | The URI of the secret. For 1Password, this can be the canonical `op://` reference or the user-friendly `op+file://<item-name>/<file-name>` for document attachments. | `"op://vault/item/secret"` or `"op+file://My Item/file.json"` |
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

### Example 2: Lease a Document Field to a File

This is useful for secrets that are files themselves, like a `.pem` key or a `google-service.json` file, and are stored in a standard `document` field in 1Password.

```toml
# env-lease.toml
[[lease]]
lease_type = "file"
source = "op://your_vault/gcp-key/document"
destination = "gcp-key.json"
duration = "8h"
```

When granted, this will create a `gcp-key.json` file in your project directory. When the lease expires, the file will be deleted.

### Example 3: Lease a Document Attachment with a User-Friendly URI

For items in 1Password that are of the "Document" category and have a file attachment, you can use the `op+file://` scheme to avoid having to look up the document's internal ID.

This scheme uses the item's name and the attached file's name.

```toml
# env-lease.toml
[[lease]]
lease_type = "file"
# Looks for an item named "app-iac container env"
# and a file attachment named "container_env.json"
source = "op+file://app-iac container env/container_env.json"
destination = "container_env.json"
duration = "1h"
```

This provides a more readable and maintainable way to reference file attachments. The content of the file can be leased to a destination file (`lease_type = "file"`) or an environment variable (`lease_type = "env"`).

---
## Limitations

### Inline Comments

`env-lease` does not support inline comments in environment files (e.g., `.env` or `.envrc`). Any inline comments on a line managed by `env-lease` will be removed when the lease is granted or revoked.

For example, this:

```
export API_KEY="some_value" # This is a comment
```

Will become this after a lease is granted and then revoked:

```
export API_KEY=""
```


---

## Security Model

For a detailed explanation of the security model, its trade-offs, and limitations, please see the [Security Model & Trade-Offs](../README.md#security-model--trade-offs) section in the main `README.md` file.
