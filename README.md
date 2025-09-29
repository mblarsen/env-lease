# env-lease

> [!IMPORTANT]
> ⚠️ **This is a toy project and is not intended for real-world use.** ⚠️
>
> This software is provided "as is" without warranty of any kind. Please do not use it to handle sensitive credentials. **Use at your own risk.**

`env-lease` is a command-line tool designed to improve the developer experience of managing secrets in local environments. It fetches secrets from a provider (like 1Password), injects them into a local file (e.g., `.envrc`), and automatically revokes them after a configurable "lease" period. This provides rapid access to secrets while minimizing the risk of leaving them scattered across your system.

### Core Features

*   **Automatic Cleanup:** Leased secrets are automatically revoked and removed after a specified duration, reducing secret sprawl.
*   **Granular Control:** Set a unique lease duration for each secret, giving you fine-grained control over its lifecycle.
*   **Flexible:** Manages both environment variables and temporary files (e.g., for service account keys).
*   **Simple & Declarative:** Configure all your secret leases in a single, easy-to-read `env-lease.toml` file.
*   **Developer-Friendly:** Get optional desktop notifications on macOS when a lease expires.

### Why?

While tools like `op read` combined with `direnv` offer a highly secure method for handling secrets by injecting them directly into the environment, this approach can introduce latency, as secrets are fetched with each new shell session. `env-lease` offers a different balance of priorities:

*   **Improved Developer Experience (DX):** By caching secrets in a local file, `env-lease` makes access nearly instantaneous, speeding up your workflow.
*   **Time-Based Leases:** To mitigate the security trade-off of writing secrets to disk, `env-lease` ensures they are automatically removed after their lease expires. You can configure different durations for each secret, giving you fine-grained control.
*   **Reduced Secret Sprawl:** By centralizing secret management in `env-lease.toml`, you can avoid leaving credentials in shell history, separate config files, or other insecure locations.

### A Note on Security

`env-lease` is designed to be a significant improvement over scattering plaintext secrets in shell history or leaving them in long-lived `.env` files. It achieves this by enforcing time-based leases that automatically clean up credentials.

However, it's important to understand the trade-offs:

*   **Filesystem vs. Memory:** `env-lease` prioritizes performance and developer experience by writing secrets to the filesystem. This is a deliberate design choice for speed. For environments requiring the highest level of security, solutions that inject secrets directly into process memory (like `op read` with `direnv`) remain the gold standard.
*   **Intended Environment:** This tool is built for trusted local development setups. It is not intended as a hardened security solution for production or other sensitive environments.


## Installation

The recommended way to install `env-lease` is via Homebrew:

```sh
brew install mblarsen/tap/env-lease
```

## Setup

First, install and start the background daemon. This is a one-time setup.

```sh
env-lease daemon install
```

## Configuration

Create a file named `env-lease.toml` in your project's root directory.

Here is a minimal example for 1Password:

```toml
# env-lease.toml
[[lease]]
source = "op://your_vault/api-key/credential"
destination = ".envrc"
variable = "API_KEY"
duration = "1h"
```

## Core Commands

### Grant a Lease

```sh
env-lease grant
```

### Check Lease Status

```sh
env-lease status
```

### Revoke a Lease

```sh
env-lease revoke
```

### Enabling Notifications on macOS

On macOS, `env-lease` can send desktop notifications when a lease expires. Due to security settings, you must manually grant permission once.

```sh
env-lease enable-notifications
```

This command will open Apple's Script Editor with a simple script. Click the "Run" button and then "Allow" the notification permission prompt. This one-time setup is all that's needed.

### Limitations

`env-lease` does not support inline comments in environment files (e.g., `.env` or `.envrc`). Any inline comments on a line managed by `env-lease` will be removed when the lease is granted or revoked.

## Security

`env-lease` uses a secure IPC model with HMAC-SHA256 token authentication to protect against unauthorized local processes interacting with its daemon.

### Limitations

The security model is designed to raise the bar for attack and prevent accidental interference. It does **not** protect against a sophisticated attacker who has already fully compromised your user account, as such an attacker could read the auth token itself.

## Next Steps

For more advanced configuration, examples, and details on the security model, please see the [Full User Documentation](docs/USAGE.md).
