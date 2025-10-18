# Plan for Deferred Secret Lookup in Interactive Grant

This document outlines the detailed plan to refactor the interactive `grant` command to implement a more efficient, multi-stage secret fetching process. The primary goal is to minimize unnecessary secret lookups and reduce latency by fetching secrets only after the user has confirmed their intent.

## Phase 1: Code Refactoring and Lease Partitioning

1.  **Isolate Interactive Logic**: The main `grantCmd.RunE` function in `cmd/grant.go` will be refactored. A new function, `interactiveGrant`, will be created to encapsulate the entire logic for the interactive mode (`--interactive` flag). The existing `RunE` logic will continue to handle the non-interactive path.

2.  **Partition Leases**: Inside `interactiveGrant`, all configured leases from `env-lease.toml` will be partitioned into three distinct groups based on their source and transformation type:
    *   **`fileLeases`**: This group will contain any lease where the `Source` URI begins with `op+file://`. These require individual processing.
    *   **`explodeLeases`**: This group will contain any `op://` lease that includes an `explode` step in its `transform` pipeline. These require a secondary confirmation step from the user.
    *   **`batchableLeases`**: This group will contain all other `op://` leases, including simple secrets and those with non-`explode` transforms (e.g., `jq`, `b64dec`).

## Phase 2: Stage 1 - Initial User Prompt

1.  **Unified Selection List**: A single, comprehensive list of choices will be constructed for the user. This list will include all leases from the `fileLeases`, `explodeLeases`, and `batchableLeases` groups.

2.  **Multi-Select Prompt**: A multi-select prompt will be presented to the user, allowing them to select all the individual secrets or secret groups they wish to grant in a single step.

## Phase 3: Stage 2 - Unified Secret Fetching

**Crucially, no secrets will be fetched before this stage.**

1.  **Compile Fetch Lists**: After the user submits their selection from the initial prompt, the choices will be processed to create final lists of secrets to be fetched.

2.  **Execute Fetch Operations**: The fetching process will be executed as follows to maximize efficiency:
    *   **File Leases**: Each selected lease from the `fileLeases` group will be fetched individually, one at a time.
    *   **Batched Leases**: The selected leases from the `batchableLeases` and `explodeLeases` groups will be combined into a single list. This combined list will then be grouped by `op_account`. A single, efficient batched fetch operation will be executed for each account group. This ensures that `batchable` and `explode` leases from the same account are fetched together in one API call.

## Phase 4: Stage 3 - Secondary Prompt for Exploded Leases

1.  **Transform Exploded Secrets**: Once the secrets for the selected `explodeLeases` have been fetched, the transformation pipeline will be run on each one to produce the final key-value data.

2.  **Secondary User Prompt**: For each `explodeLease` that was processed, a *second* multi-select prompt will be presented to the user. This prompt will display all the keys derived from the exploded secret, allowing the user to select the specific key-value pairs they want to grant.

## Phase 5: Stage 4 - Finalizing the Grant

1.  **Collect Approved Secrets**: All approved secrets will be collected into a final list:
    *   The selected `batchableLeases` and `fileLeases` with their fetched secret values.
    *   The user-selected key-value pairs from the secondary prompt for `explodeLeases`.

2.  **Process and Grant**: The existing `processLease` function and the established daemon communication logic will be used to write the secrets to their destinations and finalize the grant requests.

## Testing Strategy

- A new test file, `cmd/grant_interactive_test.go`, will be created.
- Unit tests will be added to verify the lease partitioning logic.
- A comprehensive integration test will be implemented using a mocked secret provider to simulate the entire interactive flow. This test will assert:
    - No fetch calls are made before the user's first selection.
    - `fileLeases` are fetched individually.
    - `batchableLeases` and `explodeLeases` are fetched in a single batch per account.
    - The secondary prompt is correctly displayed for exploded leases.
    - Only the final, user-approved secrets are granted.
