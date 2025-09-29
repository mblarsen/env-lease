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

Managing secrets in local development often forces a choice between convenience and security. `env-lease` exists to offer a middle ground, prioritizing a fast, uninterrupted workflow while still providing strong, automated safeguards against leaving credentials exposed long-term.

It offers a different set of trade-offs centered on:

*   **Improved Developer Experience (DX):** By caching secrets in a local file, `env-lease` makes access nearly instantaneous. This avoids the latency that can occur with tools like `op read` and `direnv`, which must fetch secrets on every new shell session.
*   **Time-Based Leases:** To mitigate the security risk of writing secrets to disk, `env-lease` ensures they are automatically removed after their lease expires. You can configure different durations for each secret, giving you fine-grained control.
*   **Reduced Secret Sprawl:** By centralizing secret management in `env-lease.toml`, you can avoid leaving credentials in shell history, separate config files, or other insecure locations.

## Security Model & Trade-Offs

`env-lease` is designed to be a significant improvement over scattering plaintext secrets in shell history or leaving them in long-lived `.env` files. It achieves this by enforcing time-based leases that automatically clean up credentials.

The CLI communicates with a background daemon via a Unix Domain Socket with restrictive file permissions. To protect against other local processes interfering with the daemon, all communication is secured using a shared secret token and **HMAC-SHA256 signatures**. This ensures that the daemon only acts on legitimate commands from the `env-lease` CLI.

However, it's important to understand the design trade-offs and limitations:

*   **Filesystem vs. Memory:** `env-lease` prioritizes performance and developer experience by writing secrets to the filesystem. For environments requiring the highest level of security, solutions that inject secrets directly into process memory (like `op read` with `direnv`) remain the gold standard.
*   **Intended Environment:** This tool is built for trusted local development setups and is not intended as a hardened security solution for production or other sensitive environments.
*   **Compromised User Account:** The security model is designed to raise the bar for attack and prevent accidental interference. It does **not** protect against a sophisticated attacker who has already fully compromised your user account, as such an attacker could read the authentication token itself.


## Usage

For installation instructions, configuration details, and a full command reference, please see the **[Full User Documentation](docs/USAGE.md)**.


