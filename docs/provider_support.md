# Provider Support

This document outlines the implementation and testing strategy for various secret providers.

### Provider Implementation & Testing Comparison

All providers can be implemented similarly to the `OnePasswordCLI` by creating a new struct that implements the `SecretProvider` interface and shells out to the respective CLI. The ease of getting started and testing varies:

| Provider | CLI Tool | Local Testing Approach | Getting Started Difficulty |
| :--- | :--- | :--- | :--- |
| **HashiCorp Vault** | `vault` | Official dev mode (`vault server -dev`) | **Easy** |
| **AWS Secrets Manager** | `aws` | `localstack` (local AWS cloud) | **Medium** |
| **Google Secret Manager**| `gcloud` | Official emulator | **Medium** |
| **Infisical** | `infisical` | Self-hosted Docker container | **Medium** |
| **Bitwarden** | `bw` | `vaultwarden` (self-hosted) | **Medium** |
| **Azure Key Vault** | `az` | No official emulator; requires mocking or a live resource | **Hard** |

### Authentication

The tool can maintain its zero-config approach by deferring authentication to the underlying CLI tools, similar to the existing 1Password integration. All the listed provider CLIs manage their own authentication state, typically through a `login` command or standard environment variables (`VAULT_TOKEN`, `AWS_ACCESS_KEY_ID`, `BW_SESSION`, etc.). `env-lease` simply needs to execute the CLI command, assuming the user has already authenticated it.
