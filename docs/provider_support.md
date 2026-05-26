# Provider Support

Provider support is split across two seams:

- `internal/provider` is the lower-level **Adapter** seam. It selects a Provider adapter from a normalized provider name, records which URI schemes the adapter can batch, and shells out to the Provider CLI.
- `internal/secretlookup` owns **Secret Lookup** orchestration. It chooses Provider/account groups before crossing the adapter seam, deduplicates singleton sources, and keeps 1Password-specific batching decisions out of `cmd` and `grantflow`.

Today the built-in registry supports 1Password (`provider = "1password"`, aliases `op` and `onepassword`). The 1Password adapter batches canonical `op://` sources through `op inject`; `op+file://` remains a 1Password adapter concern and is fetched as a deduplicated singleton because it must first resolve file metadata.

## Adding a Provider

Add one minimal adapter at a time:

1. Implement `provider.SecretProvider` in `internal/provider/<name>.go`.
2. Register an `AdapterSpec` in `provider.DefaultRegistry()` with:
   - canonical provider name and optional aliases,
   - URI schemes that are safe to fetch through `FetchLeases`,
   - an account-scoped constructor.
3. Keep account selection and cross-lease orchestration in `secretlookup`; keep CLI-specific command shape, URI normalization, and batching mechanics in the adapter.
4. Add registry tests for unknown provider errors, selection, and scheme batching. Add adapter tests for CLI command behaviour.

Avoid adding Provider-specific branches in `cmd` or `grantflow`. If adding a Provider requires those callers to know a URI scheme or CLI detail, deepen the `provider` registry or `secretlookup` plan instead.

## Provider Implementation & Testing Comparison

All providers can be implemented similarly to `OnePasswordCLI`: create a struct that implements `SecretProvider`, shell out to the respective CLI, and register its schemes. The ease of getting started and testing varies:

| Provider | CLI Tool | Local Testing Approach | Getting Started Difficulty |
| :--- | :--- | :--- | :--- |
| **HashiCorp Vault** | `vault` | Official dev mode (`vault server -dev`) | **Easy** |
| **AWS Secrets Manager** | `aws` | `localstack` (local AWS cloud) | **Medium** |
| **Google Secret Manager** | `gcloud` | Official emulator | **Medium** |
| **Infisical** | `infisical` | Self-hosted Docker container | **Medium** |
| **Bitwarden** | `bw` | `vaultwarden` (self-hosted) | **Medium** |
| **Azure Key Vault** | `az` | No official emulator; requires mocking or a live resource | **Hard** |

## Authentication

The tool can maintain its zero-config approach by deferring authentication to the underlying CLI tools, similar to the existing 1Password integration. Provider CLIs manage their own authentication state, typically through a login command or standard environment variables (`VAULT_TOKEN`, `AWS_ACCESS_KEY_ID`, `BW_SESSION`, etc.). `env-lease` executes the CLI command assuming the user has already authenticated it.
