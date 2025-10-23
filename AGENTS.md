# Repository Guidelines

## Project Structure & Module Organization
Source lives under `cmd/env-lease` for the CLI entrypoint and `internal` for reusable packages (e.g., `internal/daemon`, `internal/provider`). Shared helpers—config parsing, IPC, transforms—follow the same folder name as their Go package. Assets for docs and tests live in `assets/`, while distributable binaries are written to `bin/` and `dist/` when you build. Task Master metadata is stored under `.taskmaster/` (`tasks/tasks.json`, generated task notes, and PRDs); commit those files only through the Task Master workflow. Configuration examples live as `.toml` files in the repo root.

## Build, Test, and Development Commands
Prefer `just` recipes for repeatability: `just build` produces `./bin/env-lease`, `just test` runs `go test ./...`, and `just lint` wraps `go vet` (enable `golangci-lint` locally if you need stricter checks). Use `go run ./cmd/env-lease` for quick manual testing without rebuilding, and `just fmt` to apply `gofmt`/`goimports` project-wide.

## Task Master Workflow
Coordinate work manually or via `task-master list` and `task-master show <task>`—do not use `task-master next`. As soon as you pick a task, mark it in progress (`task-master set-status --id=<task> --status=in-progress`). After implementing and validating, mark it done (`task-master set-status --id=<task> --status=done`) before committing. The status transition updates `.taskmaster/tasks/tasks.json`; include that file in the commit and never edit it or `.taskmaster/config.json` by hand. Use Task Master subtask commands for adjustments instead of editing generated files manually.

## Coding Style & Naming Conventions
Use `gofmt`’s default formatting and keep imports grouped by standard, third-party, then internal packages. Name packages in lowercase, unexported identifiers in camelCase, and exported APIs in PascalCase. Exported symbols need doc comments that begin with the identifier name. Avoid stutter by letting packages convey context (`daemon.Manager`, not `DaemonManager`), and return `error` as the final value—handle it explicitly rather than discarding. Tests should mirror the files they cover and end with `_test.go`.

## Testing Guidelines
Unit and integration tests rely on the standard `testing` package with helpers in `internal/...`. Name tests with `Test<Type><Scenario>` (e.g., `TestLeaseGrant_File`). Run `go test ./...` before opening a PR; add focused checks with `just test-one TestLeaseGrant_File`. Use table-driven patterns for multi-case logic, stub interfaces over concrete types, and clean up any filesystem artifacts created during tests.

## Commit & Pull Request Guidelines
Follow the existing history: prefix messages with a scope (`docs:`, `test:`, `refactor:`) and keep the subject under ~72 characters. Each commit should be buildable, include the Task Master status update (`.taskmaster/tasks/tasks.json`), and stay focused on one concern. Pull requests need a brief summary, testing notes (`just test` output), and links to relevant issues or docs. Reference the Task Master task ID in the PR and include screenshots or CLI transcripts when modifying user-visible behavior.

## Security & Configuration Tips
Never commit real secrets; keep sample configuration in `env-lease.alt.toml` style files and document new keys in `docs/`. When touching secret providers or token handling, call out threat-model assumptions in the PR description and add regression tests under the matching `internal/provider` package. Do not re-initialize Task Master—reuse the existing configuration and store new API keys in `.env`.
