# env-lease

> [!WARNING]
> Disclaimer: This is a toy project and is not intended for use. Please do not use this software.

A command-line tool for managing temporary, leased secrets in local development environment files. It fetches secrets from a backend (like 1Password), writes them to a file, and automatically clears them after a specified "lease" duration expires.

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

## Security

`env-lease` uses a secure IPC model with HMAC-SHA256 token authentication to protect against unauthorized local processes interacting with its daemon.

### Limitations

The security model is designed to raise the bar for attack and prevent accidental interference. It does **not** protect against a sophisticated attacker who has already fully compromised your user account, as such an attacker could read the auth token itself.

## Next Steps

For more advanced configuration, examples, and details on the security model, please see the [Full User Documentation](docs/USAGE.md).
