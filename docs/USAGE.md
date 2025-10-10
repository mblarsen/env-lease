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

## Configuration (`env-lease.toml`)

The `env-lease.toml` file is the heart of the configuration. It's a declarative file that defines all the leases for a project.

### Advanced Configuration

In addition to the `env-lease.toml` file, you can use environment variables to control the configuration:

- `ENV_LEASE_CONFIG`: Specifies the full path to the configuration file.
- `ENV_LEASE_NAME`: Specifies the name of the configuration file, which is then looked for in the current directory.

The order of precedence is:

1.  `--config` flag
2.  `ENV_LEASE_CONFIG`
3.  `ENV_LEASE_NAME`
4.  `env-lease.toml` (default)


### Lease Options

| Key           | Required | Description                                                                                                                                                          | Example                                                       |
| ------------- | -------- | -------------------------------------------------------------------------------------------------------------------------------------------------------------------- | ------------------------------------------------------------- |
| `source`      | Yes      | The URI of the secret. For 1Password, this can be the canonical `op://` reference or the user-friendly `op+file://<item-name>/<file-name>` for document attachments. | `"op://vault/item/secret"` or `"op+file://My Item/file.json"` |
| `destination` | Yes\*    | The relative path to the target file. _Required for `env` and `file` types only._                                                                                    | `".envrc"`                                                    |
| `duration`    | Yes      | The lease duration (e.g., "10m", "1h", "8h").                                                                                                                        | `"8h"`                                                        |
| `lease_type`  | No       | The type of lease. Can be `"env"` (default), `"file"`, or `"shell"`.                                                                                                 | `"shell"`                                                     |
| `variable`    | Yes\*    | The name of the environment variable to set. _Required for `env` and `shell` types._                                                                                 | `"API_KEY"`                                                   |
| `format`      | No       | A Go `sprintf`-style format string for `env` leases. Defaults are applied for `.env` and `.envrc`.                                                                   | `"export %s=%q"`                                              |
| `transform`   | No       | An array of transformations to apply to the secret before writing it. See the "Transformations" section below.                                                       | `["base64-decode", "json", "select 'key'"]`                   |
| `op_account`  | No       | The 1Password account to use. Overrides the `OP_ACCOUNT` environment variable.                                                                                       | `"my-account"`                                                |

## Secret Transformations

The `transform` option provides a powerful pipeline to process secrets after they are fetched but before they are written to a file. This is ideal for handling secrets that are not plain text, such as base64-encoded values or structured data like JSON or YAML.

The `transform` key accepts an array of strings, where each string represents a single step in the pipeline. The secret is passed through each step in order.

### The Pipeline Flow

The transformation pipeline is type-aware. It starts with a `string` (the raw secret from the provider), but its internal data type can change from one step to the next.

1.  **Initial State:** The pipeline starts with a `string`.
2.  **Parsing (Optional):** Transformers like `json`, `toml`, or `yaml` parse the input string into an internal, structured data format.
3.  **Querying (Optional):** The `select` transformer operates on this structured data to extract a specific value. Its output can be a `string` or another `structured_data` object.
4.  **Final State:** The final step in the pipeline **must** produce a `string` (for a single lease) or `exploded_data` (for multiple leases via the `explode` transformer).

For example, you cannot have `json` as the last step, because its output is structured data. It must be followed by a `select` or `explode` step.

### Transformer Reference

| Transformer       | Description                                                                                                                                               | Input Type        | Output Type                   |
| :---------------- | :-------------------------------------------------------------------------------------------------------------------------------------------------------- | :---------------- | :---------------------------- |
| `base64-encode`   | Encodes the input string to standard base64.                                                                                                              | `string`          | `string`                      |
| `base64-decode`   | Decodes a base64-encoded input string.                                                                                                                    | `string`          | `string`                      |
| `json`            | Parses a valid JSON string into structured data.                                                                                                          | `string`          | `structured_data`             |
| `toml`            | Parses a valid TOML string into structured data.                                                                                                          | `string`          | `structured_data`             |
| `yaml`            | Parses a valid YAML string into structured data.                                                                                                          | `string`          | `structured_data`             |
| `select '<path>'` | Extracts a value from structured data using dot-notation. The path must be quoted.                                                                        | `structured_data` | `string` or `structured_data` |
| `explode(...)`    | **Terminating transform.** Expands structured data into multiple variables. Accepts optional `filter` and `prefix` arguments. Must be the last transform. | `structured_data` | `exploded_data`               |
| `to_json`         | Converts structured data to a JSON string.                                                                                                                | `structured_data` | `string`                      |
| `to_yaml`         | Converts structured data to a YAML string.                                                                                                                | `structured_data` | `string`                      |
| `to_toml`         | Converts structured data to a TOML string.                                                                                                                | `structured_data` | `string`                      |

### Example: Extracting a Nested Value from JSON

This is a common use case where a single 1Password item stores multiple related values.

**Secret stored in 1Password:**

```json
{
  "database": {
    "user": "admin",
    "pass": "p@ssw0rd-123"
  },
  "api_key": "abc-123"
}
```

**`env-lease.toml` configuration:**
To extract just the database password, you would define the following pipeline:

```toml
[[lease]]
source = "op://vault/item/my-json-secret"
destination = ".envrc"
variable = "DB_PASSWORD"
duration = "8h"
# 1. Parse the incoming secret as JSON.
# 2. Select the value at the path 'database.pass'.
transform = ["json", "select 'database.pass'"]
```

### Example: Expanding a JSON Object with `explode`

The `explode` transform is a powerful way to turn a single structured secret into multiple environment variables. It accepts two optional, named arguments:

- `filter=PREFIX_`: Only processes keys that already start with `PREFIX_`.
- `prefix=PREFIX_`: Adds `PREFIX_` to the beginning of every processed key.

**Secret stored in 1Password:**

```json
{
  "ORY_API_KEY": "key-12345",
  "ORY_API_SECRET": "secret-abcde",
  "AWS_REGION": "us-east-1"
}
```

**`env-lease.toml` Examples:**

**1. Filter Only:**
To get only the `ORY_` variables:

```toml
transform = ["json", "explode(filter=ORY_)"]
# Result: ORY_API_KEY, ORY_API_SECRET
```

**2. Prefix Only:**
To add `MYAPP_` to all variables:

```toml
transform = ["json", "explode(prefix=MYAPP_)"]
# Result: MYAPP_ORY_API_KEY, MYAPP_ORY_API_SECRET, MYAPP_AWS_REGION
```

**3. Filter and Prefix:**
To get only the `ORY_` variables and then add a `REACT_` prefix to them:

```toml
transform = ["json", "explode(filter=ORY_, prefix=REACT_)"]
# Result: REACT_ORY_API_KEY, REACT_ORY_API_SECRET
```

**4. No Arguments:**
To get all variables as they are:

```toml
transform = ["json", "explode"]
# Result: ORY_API_KEY, ORY_API_SECRET, AWS_REGION
```

**Safety Blacklist:** To prevent accidental clobbering of critical system variables, `env-lease` enforces a blacklist on the _final_ generated variable name. If a generated variable is blacklisted (e.g., `PATH`), the grant operation will fail.

> **Note:** The `explode` transform can only be used with `lease_type = "env"` or `lease_type = "shell"`. It cannot be used to create multiple files.

## Scaffolding Configuration from `.env`

For existing projects that already use a `.env` or `.envrc` file with 1Password URIs, you can use the `convert` command to quickly generate a starting `env-lease.toml` configuration.

Given a `.envrc` file like this:

```sh
# .envrc
export DATABASE_URL="op://vault/item/db-url"
export API_KEY="op://vault/item/api-key"
```

You can generate a configuration by running:

```sh
env-lease convert .envrc > env-lease.toml
```

This will produce an `env-lease.toml` file with leases for `DATABASE_URL` and `API_KEY`. You will still need to edit the file to set the desired `duration` for each lease.

## Automatic Revocation on Idle

For enhanced security, `env-lease` can be configured to automatically revoke all active leases after a period of user inactivity. This is managed by a background service that periodically checks for system idle time.

This feature is supported on both macOS (via `launchd`) and Linux (via `systemd`).

### Enabling the Idle Revocation Service

To install and start the service, use the `idle install` command. You can customize the idle timeout and how frequently the check runs.

```sh
# Install with a 30-minute idle timeout and a 5-minute check interval (default)
env-lease idle install --timeout 30m

# Install with a 1-hour timeout and a check every 2 minutes
env-lease idle install --timeout 1h --check-interval 2m
```

### Checking the Service Status

You can verify that the service is installed and running with the `idle status` command:

```sh
env-lease idle status
```

### Disabling the Service

To stop the service and remove all its components, use the `idle uninstall` command:

```sh
env-lease idle uninstall
```

## Command Reference

| Command                          | Description                                                                              |
| -------------------------------- | ---------------------------------------------------------------------------------------- |
| `env-lease grant`                | Grants all leases defined in `env-lease.toml`.                                           |
| `env-lease revoke`               | Immediately revokes all secrets defined in the current project's `env-lease.toml`.       |
| `env-lease status`               | Lists all currently active leases managed by the daemon.                                 |
| `env-lease convert`              | Scaffolds an `env-lease.toml` file from an existing `.env` or `.envrc` file.             |
| `env-lease enable-notifications` | (macOS only) Guides the user to grant notification permissions.                          |
| `env-lease daemon install`       | Installs and starts the daemon as a user service.                                        |
| `env-lease daemon uninstall`     | Stops and uninstalls the daemon.                                                         |
| `env-lease daemon status`        | Checks the status of the daemon service.                                                 |
| `env-lease daemon reload`        | Reloads the daemon service.                                                              |
| `env-lease daemon cleanup`       | Manually purges all orphaned leases from the daemon's state.                             |
| `env-lease idle install`         | Installs and starts the idle revocation service. Flags: `--timeout`, `--check-interval`. |
| `env-lease idle uninstall`       | Stops and uninstalls the idle revocation service.                                        |
| `env-lease idle status`          | Checks the status of the idle revocation service.                                        |

### Command Flags

#### `grant`

- `--override`: Re-grant leases even if they are already active.
- `--config`: Path to the configuration file. This can be overridden by the `ENV_LEASE_CONFIG` and `ENV_LEASE_NAME` environment variables.
- `--continue-on-error`: Continue granting leases even if one fails.
- `-i`, `--interactive`: Prompt for confirmation before granting each lease.
- `--destination-outside-root`: Allow file-based leases to write outside of the project root.

#### `revoke`

- `--all`: Revoke all active leases, regardless of which project they belong to.
- `-i`, `--interactive`: Prompt for confirmation before revoking each lease.

### Interactive Mode

The `-i` or `--interactive` flag can be used with `grant` and `revoke` to confirm each action individually. In this mode, you have several options:

- `y`: Yes, perform this action.
- `n` or `<Enter>`: No, skip this action.
- `a`: Yes to this and all subsequent actions in this run.
- `d`: No to this and all subsequent actions in this run.
- `?`: Show help.

**Granting Leases:**

```sh
$ env-lease grant -i
Grant lease for 'GOOGLE_API_KEY'? [y/n/a/d/?] y
Grant lease for 'OPENAI_API_KEY'? [y/n/a/d/?] n
```

**Revoking Leases:**

```sh
$ env-lease revoke -i
Revoke lease for 'GOOGLE_API_KEY'? [y/n/a/d/?] a
```

> [!NOTE]
> Interactive mode is not supported for `shell` type leases, as they require being run inside `eval $(...)` which is non-interactive.

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

### Example 4: Lease a Variable Directly to Your Shell

The `shell` lease type allows you to export a secret directly as an environment variable in your current shell session, without needing a tool like `direnv`.

This is useful for quick, temporary access to credentials.

```toml
# env-lease.toml
[[lease]]
lease_type = "shell"
source = "op://your_vault/your_item/your_field"
variable = "QUICK_API_KEY"
duration = "15m"
```

**Granting the Lease**

To grant a `shell` lease, you must wrap the command in `eval $()` so your shell can process the `export` command that `env-lease` outputs:

```sh
eval $(env-lease grant)

# Now you can use the variable
echo $QUICK_API_KEY
```

**Revoking the Lease**

Similarly, to `unset` the variable from your shell, you must use `eval $()` with the `revoke` command:

```sh
eval $(env-lease revoke)
```

> **Note on Automatic Revocation:** While the lease is tracked by the `env-lease` daemon and expires automatically, the environment variable will **remain in your shell** after expiration. You must manually run `eval $(env-lease revoke)` or close the shell session to remove it. This is a fundamental limitation of how shell environments work.

## Upgrading

**Important:** Before upgrading to a new version of `env-lease`, especially during this pre-release stage of development, it is crucial to revoke all active leases.

Due to the unstable nature of the software, breaking changes to the daemon's state or communication protocol may occur between versions. To prevent orphaned leases or other issues, run the following command **before** you stop the daemon or replace the application binary:

```sh
env-lease revoke --all
```

This will ensure a clean state before you upgrade.

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

## Security Model

For a detailed explanation of the security model, its trade-offs, and limitations, please see the [Security Model & Trade-Offs](../README.md#security-model--trade-offs) section in the main `README.md` file.
