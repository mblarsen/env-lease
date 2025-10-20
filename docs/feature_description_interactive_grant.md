# Feature Description: `grant --interactive`

This document provides a detailed description of the `env-lease grant --interactive` command, explaining its user-facing workflow, internal mechanics, and the design choices that ensure a secure, efficient, and user-friendly experience.

## Overview

The `grant --interactive` command allows users to review and approve each secret lease individually before it is granted. This provides granular control, preventing accidental leasing of sensitive credentials and offering a clear audit point for temporary access.

The core of this feature is a **deferred secret lookup** mechanism. Instead of fetching all secrets from the provider (e.g., 1Password) upfront, `env-lease` waits until the user explicitly approves a lease. This design choice minimizes unnecessary access to sensitive data and significantly speeds up the initial interaction.

## Key Design Principles

1.  **Just-in-Time Secret Fetching**: Secrets are only retrieved from the provider *after* the user approves the corresponding lease.
2.  **Intelligent Batching**: To minimize latency, approved `op://` leases sharing the same `op_account` are fetched together in a single, batched `op` CLI call.
3.  **Efficient Caching**: `op+file://` sources are fetched only once per run. If multiple leases use the same `op+file://` URI, the content is fetched for the first approved lease and then reused from an in-memory cache for all subsequent leases in the same run.
4.  **Strictly Ordered Workflow**: The interactive flow is separated into distinct, predictable phases: a complete pass for approving sources (Round 1), a parallelized fetching phase, and a final pass for approving individual secrets from `explode` leases (Round 2). This ensures a consistent user experience.
5.  **Descriptive & Multi-Stage Prompting**: For leases with an `explode` transform, the user is guided through a two-stage approval process. To avoid ambiguity, prompts for such leases include details from the transformation pipeline (e.g., `select 'production'`), ensuring the user knows exactly which configuration they are approving.

## The User Workflow

When a user runs `env-lease grant -i` or `env-lease grant --interactive`, they experience a clean, multi-phase flow:

### Phase 1: Round 1 - Approve Sources

The command first makes a complete pass through the `env-lease.toml` configuration, prompting the user to approve or deny each top-level `[[lease]]` block. **No secrets are fetched during this phase.** This round is solely for approving the *sources* of the secrets.

The user interacts with a simple prompt for each item:

```
Grant lease for 'GOOGLE_API_KEY'? [y/n/a/d/?]
```

### Phase 2: Fetch Secrets

Once Round 1 is complete, the system executes all necessary secret lookups for the approved sources. To maximize speed, these lookups are performed **in parallel**:

*   One batched `op` call is made for each group of approved `op://` leases that share an `op_account`.
*   One individual `op` call is made for each unique `op+file://` URI that was approved.

### Phase 3: Round 2 - Approve Individual Secrets (Optional)

This is an optional phase that only runs for `explode` leases that were approved in Round 1. All simple (non-`explode`) leases approved in the first round are now considered final and are ready to be granted without any further prompts.

For each approved `explode` lease, the user is now prompted to approve or deny the individual key-value pairs from within the fetched source.

### Phase 4: Grant Leases

Once all approvals are gathered, the final, verified list of leases is sent to the `env-lease` daemon to be activated.

## Simulation

This section provides a detailed, step-by-step walkthrough of the interactive grant flow using the example configuration. It demonstrates the improved logic for descriptive prompts and efficient, cached secret lookups that were refined during the simulation.

> **Discovery**: The most logical and consistent user experience is to complete all of Round 1 (approving sources) before performing any secret lookups or starting Round 2 (approving secrets within sources). This keeps the workflow predictable, regardless of the underlying source type (`op://` or `op+file://`).

### Round 1: Approving Sources

The system makes a complete pass through all `[[lease]]` blocks, collecting approvals for each source.

1.  **Lease**: `GOOGLE_API_KEY` (`op_account="plantura"`)
    *   **Prompt**: `Grant lease for 'GOOGLE_API_KEY'?`
    *   **Response**: `y`
2.  **Lease**: `HOMEBREW_GITHUB_API_TOKEN` (`op_account="plantura"`)
    *   **Prompt**: `Grant lease for 'HOMEBREW_GITHUB_API_TOKEN'?`
    *   **Response**: `y`
3.  **Lease**: `OPENAI_API_KEY` (`op_account="my"`)
    *   **Prompt**: `Grant lease for 'OPENAI_API_KEY'?`
    *   **Response**: `y`
4.  **Lease**: `OPENROUTER_API_KEY` (`op_account="my"`)
    *   **Prompt**: `Grant lease for 'OPENROUTER_API_KEY'?`
    *   **Response**: `n`
5.  **Lease**: `op+file://...` (Production)
    *   **Refined Prompt**: `Grant leases from 'op+file://app-iac container env/container_env.json' (select 'production', explode(filter=ORY_))?`
    *   **Response**: `y`
6.  **Lease**: `op+file://...` (Development)
    *   **Refined Prompt**: `Grant leases from 'op+file://app-iac container env/container_env.json' (select 'development', explode(filter=ORY_, prefix=DEV_))?`
    *   **Response**: `y`

### Secret Fetching Phase

After Round 1 is complete, the system executes all necessary `op` calls based on the approvals. These three calls are performed **in parallel**.

1.  **`op` Call #1 (Batched):**
    *   **Reason**: Processes the approved `plantura` account batch.
    *   **Fetches**: `GOOGLE_API_KEY` and `HOMEBREW_GITHUB_API_TOKEN`.
2.  **`op` Call #2 (Batched):**
    *   **Reason**: Processes the approved `my` account batch.
    *   **Fetches**: `OPENAI_API_KEY`.
3.  **`op` Call #3 (Individual):**
    *   **Reason**: Processes the first approved `op+file://` source.
    *   **Fetches**: The content of `container_env.json`.
    *   **Caching**: The fetched content is cached for the remainder of the command.

### Round 2: Approving Individual Secrets from `explode`

Now the system circles back to the `explode` leases that were approved in Round 1 and prompts for the individual keys.

*   **Source**: `op+file://...` (Production)
    *   **Prompt**: `From '.../container_env.json' (production): Grant lease for 'ORY_API_KEY'?`
    *   **Response**: `y`
    *   **Prompt**: `From '.../container_env.json' (production): Grant lease for 'ORY_API_SECRET'?`
    *   **Response**: `n`
*   **Source**: `op+file://...` (Development)
    *   The system uses the **cached content** of `container_env.json` for this step.
    *   **Prompt**: `From '.../container_env.json' (development): Grant lease for 'DEV_ORY_API_KEY'?`
    *   **Response**: `y`
    *   **Prompt**: `From '.../container_env.json' (development): Grant lease for 'DEV_ORY_API_SECRET'?`
    *   **Response**: `y`

### Final Leases to be Granted

The following list of secrets, along with their destinations, would be sent to the `env-lease` daemon for activation:

*   **To `.envrc`:**
    *   `GOOGLE_API_KEY`
    *   `HOMEBREW_GITHUB_API_TOKEN`
    *   `OPENAI_API_KEY`
*   **To `.env`:**
    *   `ORY_API_KEY` (from the production `op+file` lease)
    *   `DEV_ORY_API_KEY` (from the development `op+file` lease)
    *   `DEV_ORY_API_SECRET` (from the development `op+file` lease)
