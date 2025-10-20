# Refactoring Plan for `grant --interactive`

This document outlines the test-driven development (TDD) plan for refactoring the `interactiveGrant` function in `cmd/grant.go` to match the correct multi-phase workflow.

## The Plan

1.  **Cleanup `cmd/grant_test.go`:**
    *   Remove the two simple interactive tests (`t.Run("interactive mode", ...)` and `t.Run("interactive mode with explode", ...)`) to centralize interactive testing in a single file.

2.  **Add a "Minimum Viable" Multi-Phase Test:**
    *   Add a new test case to `cmd/grant_interactive_test.go`.
    *   This test will use a simple but effective configuration: one simple (non-`explode`) lease and one `explode` lease.
    *   The test's primary purpose is to validate the strict ordering of the interactive phases. It must:
        *   Assert that the prompts for the simple lease and the *source* of the explode lease appear first (verifying Round 1).
        *   Assert that the prompts for the *keys inside* the explode lease appear last (verifying Round 2).
    *   This test is expected to **fail** with the current implementation.

3.  **Refactor `cmd/grant.go`:**
    *   With the new, failing test in place as a safety net, refactor the `interactiveGrant` function.
    *   The implementation must be changed from the current single-loop logic to the correct, multi-phase workflow as detailed in the `TODO` comment within the function.
    *   After the refactor, the new test case added in Step 2 should **pass**.

4.  **Expand Test Suite:**
    *   Once the refactor is complete and verified by the minimal test, add more complex test cases to `cmd/grant_interactive_test.go`.
    *   These new tests will cover the other nuances we've designed, such as:
        *   Correct batching for multiple `op_account`s.
        *   Efficient caching of `op+file://` sources.
        *   Handling a mix of "yes" and "no" answers across both rounds.
        *   Verifying that the correct final set of secrets is written to the correct destination files.

## Detailed Implementation Design

This section details the specific changes required to correctly implement the provider contract and the `interactiveGrant` fetching logic.

### 1. The Provider Contract (`internal/provider/`)

The core issue is that the return value of `FetchLeases` is ambiguous. We will formalize a new, robust contract.

*   **`provider.go`**: The comment for the `SecretProvider` interface will be updated to explicitly state the new contract for `FetchLeases`:
    *   The returned `map[string]string` **MUST** be keyed by the lease's unique **`source` URI**.
    *   This ensures a stable key that works for all lease configurations (simple, file, and `explode`).

*   **`onepassword.go`**: The `FetchLeases` implementation will be modified to adhere to this new contract.
    *   After making the single, batched call to the `op` CLI, it will construct a map where the keys are the `lease.Source` URIs from the input slice, not the `lease.Variable` names.

*   **`mock.go`**: The mock `FetchLeases` implementation will also be updated to return a map keyed by `lease.Source`. This is critical for ensuring our test suite is consistent with the real implementation.

### 2. The Caller Implementation (`cmd/grant.go`)

The `interactiveGrant` function will be updated to correctly manage and look up secrets fetched under the new provider contract.

*   **Data Structure:** A nested map will be used to store fetched secrets, ensuring uniqueness across different accounts:
    ```go
    // map[account_name] -> map[source_uri] -> secret_value
    fetchedSecretsByAccount := make(map[string]map[string]string)
    ```
    A separate map will handle `op+file://` sources, which don't have accounts but are keyed by their unique source URI.
    ```go
    // map[source_uri] -> secret_value
    fetchedFileSecrets := make(map[string]string)
    ```

*   **Fetching Logic (Phase 2):**
    *   The function will iterate over the `opLeasesByAccount` groups.
    *   For each `account` and `leasesInBatch`, it will call `p.FetchLeases(leasesInBatch)`.
    *   The resulting `map[source]secret` will be stored in `fetchedSecretsByAccount[account]`.
    *   It will then iterate over the unique `fileLeasesBySource`, fetch each one individually, and store the result in `fetchedFileSecrets`.

*   **Processing Logic (Phase 4):**
    *   When processing the final list of `approvedSources`, the secret lookup will be unambiguous.
    *   For an `op://` lease, the value will be retrieved with:
        `secret := fetchedSecretsByAccount[lease.OpAccount][lease.Source]`
    *   For an `op+file://` lease, the value will be retrieved with:
        `secret := fetchedFileSecrets[lease.Source]`
    *   This ensures the correct secret is always used, even if the same `source` URI exists in multiple accounts.
