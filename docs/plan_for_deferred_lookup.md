# Plan for Deferred Secret Lookup in Interactive Grant

This document outlines the refactored design for the interactive `grant` command, implementing an efficient, multi-stage secret fetching process. The primary goals are to enhance user experience by deferring secret lookups until they are approved and to minimize latency by intelligently batching calls to the `op` CLI.

### Core Principles

1.  **Decoupled Source & Destination**: A lease's source (`op://` or `op+file://`) is independent of its destination (e.g., environment variable or file).
2.  **`op://` Sources Are Always Batchable**: All leases originating from a standard `op://` source are eligible for batch lookup, *including those with an `explode` transform*. The only constraint is that they must share the same `op_account`.
3.  **`op+file://` Sources Are Never Batchable**: Leases from an `op+file://` source must always be fetched individually. This is the primary distinction that dictates the lookup strategy.
4.  **Efficient, Just-in-Time Batching**: The system is smart about *when* it performs a batch lookup. The first time an approved lease from a specific `op_account` needs its secret, the system fetches the secrets for **all other approved leases** from that same account at the same time. This prevents redundant batch calls.

### The Interactive Grant Flow

When a user runs `env-lease grant --interactive`, the following flow is executed:

**Step 1: Initial Unified Prompt**

The command first categorizes all `[[lease]]` configurations to build a single, unified list of items for the user to approve or deny. At this stage, no secrets have been fetched.

-   The prompt for a simple `op://` lease is: `Grant lease for 'STRIPE_KEY'?`
-   The prompt for an `op://` lease with `explode` is: `Grant leases from 'op://team/secrets'?`
-   The prompt for an `op+file://` lease is: `Grant leases from 'op+file://config/dev-env'?`

The user makes their selections, creating an "Approved Set" of leases.

**Step 2: Process Approved Leases and Fetch Secrets**

The system iterates through the "Approved Set" to determine the most efficient fetch strategy.

-   **If the lease is from `op+file://`:**
    -   An **immediate, individual `op` CLI call** is made to fetch the file's content.
    -   If the lease has an `explode` transform, the content is parsed, and the user is immediately presented with a **second round of prompts** for the key-value pairs within.

-   **If the lease is from `op://`:**
    -   The system checks if the secrets for its `op_account` have already been fetched.
    -   **If not**, it triggers a **single, batched `op` CLI call**. This call fetches the secret for the current lease *and* for every other approved lease sharing that same `op_account`. The account is then marked as "fetched."
    -   If the lease has an `explode` transform, the now-available secret is parsed, and the user is presented with the **second round of prompts**.

**Step 3: Final Granting**

After all approved leases have been processed and all necessary secrets fetched, the final, complete list of approved secrets and their destinations is sent to the `env-lease` daemon.

### Use Case Example

This scenario demonstrates the efficient, corrected flow with a mix of lease types.

**Configuration:**

```toml
# Prod Account Leases
[[lease]] # Batchable
source = "op://vault/stripe-key/credential"
variable = "STRIPE_KEY"
op_account = "prod"

[[lease]] # Batchable Explode
source = "op://vault/prod-env/document"
transform = ["json", "explode"]
op_account = "prod"

# Dev Account Lease
[[lease]] # Batchable
source = "op://vault/dev-key/credential"
variable = "DEV_KEY"
op_account = "dev"

# File-Source Lease (no account needed)
[[lease]] # Individual Explode
source = "op+file://vault/local-settings/document"
transform = ["json", "explode"]
```

**Interactive Flow (`--interactive`):**

1.  **Round 1 Prompts**: The user sees four prompts for `STRIPE_KEY`, `op://vault/prod-env/document`, `DEV_KEY`, and `op+file://vault/local-settings/document`. They approve all.

2.  **Processing and Fetching**:
    -   The system processes the first approved "prod" lease (`STRIPE_KEY`). It sees the "prod" account's secrets are not yet fetched.
    -   A **single batched `op` call** is made for the "prod" account, fetching secrets for both `STRIPE_KEY` and `op://vault/prod-env/document`.
    -   The system proceeds to the `op://vault/prod-env/document` lease. Since its secret is now available, it immediately triggers the **Round 2 Prompts** for the keys inside the document.
    -   Next, it processes the `DEV_KEY` lease. As this is a new account, a **second batched `op` call** is made for the "dev" account.
    -   Finally, it processes the `op+file://` lease. It makes a **third, individual `op` call** to fetch the file and then shows the **Round 2 Prompts** for its contents.

3.  **Granting**: All approved secrets are sent to the daemon.

This entire process is completed in just **3 `op` CLI calls**, demonstrating the efficiency of the design.
