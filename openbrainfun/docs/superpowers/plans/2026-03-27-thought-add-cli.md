# Thought Add CLI Implementation Plan

> **For agentic workers:** REQUIRED: Use superpowers:subagent-driven-development (if subagents available) or superpowers:executing-plans to implement this plan. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Add `openbrain thought add` so operators can create thoughts directly from local env-backed database config.

**Architecture:** Extend the existing `cmd/openbrain` command tree with a new `thought` family, initially implementing only `thought add`. Reuse existing auth and thought services directly against the configured Postgres database so CLI-created thoughts follow the same validation and ingest lifecycle as web/API-created thoughts.

**Tech Stack:** Go, pgxpool, existing `internal/auth`, `internal/thoughts`, `internal/postgres` services/repositories.

---

## Chunk 1: CLI surface and parser

### Task 1: Add failing parser/execution tests

**Files:**
- Modify: `cmd/openbrain/cli_test.go`
- Test: `cmd/openbrain/cli_test.go`

- [ ] **Step 1: Write the failing test** for successful `thought add` invocation with username, content, exposure scope, and repeated tags.
- [ ] **Step 2: Run `go test ./cmd/openbrain -run 'TestExecuteThoughtAdd'`** and verify failure for missing command support.
- [ ] **Step 3: Write additional failing tests** for required username/content and usage text.
- [ ] **Step 4: Re-run the focused test command** and verify the expected failures.

### Task 2: Implement command parsing

**Files:**
- Modify: `cmd/openbrain/cli.go`

- [ ] **Step 1: Add `thought` command routing** in `execute` and usage text.
- [ ] **Step 2: Add `thought add` argument parsing** with positional `<username> <content>` plus `--exposure-scope` and repeated `--tag`.
- [ ] **Step 3: Keep parsing strict**: unknown flags, missing values, missing operands, and extra operands should error clearly.
- [ ] **Step 4: Run `go test ./cmd/openbrain -run 'TestExecuteThoughtAdd|TestExecuteWithoutArgsShowsUsage'`** and verify green.

## Chunk 2: Runtime wiring and direct creation

### Task 3: Add failing runner-level tests

**Files:**
- Modify: `cmd/openbrain/cli_test.go`

- [ ] **Step 1: Expand the fake dependencies** so tests can observe direct thought creation input.
- [ ] **Step 2: Add a failing execution test** asserting the command resolves the username and calls the thought creation path with normalized tags/exposure.
- [ ] **Step 3: Run the focused test** and verify it fails for missing implementation.

### Task 4: Implement direct thought creation wiring

**Files:**
- Modify: `cmd/openbrain/cli.go`
- Modify: `cmd/openbrain/main.go` (only if helper placement is cleaner there)

- [ ] **Step 1: Add a thought runner dependency** that builds from local env config with pgxpool plus existing postgres/auth/thought services.
- [ ] **Step 2: Resolve username via auth store** and call `thoughts.Service.CreateThought` with the parsed fields.
- [ ] **Step 3: Print a compact success message** including thought ID, username, exposure scope, tags, and ingest status.
- [ ] **Step 4: Run `go test ./cmd/openbrain`** and verify all CLI tests pass.

## Chunk 3: Final verification

### Task 5: Verify and review

**Files:**
- Modify: `cmd/openbrain/cli.go`
- Modify: `cmd/openbrain/cli_test.go`
- Possibly modify: `cmd/openbrain/main.go`

- [ ] **Step 1: Run `gofmt -w cmd/openbrain/cli.go cmd/openbrain/cli_test.go cmd/openbrain/main.go`** on touched files.
- [ ] **Step 2: Run `GOCACHE=/tmp/openbrainfun-go-cache go test ./cmd/openbrain`**.
- [ ] **Step 3: Inspect `git diff -- cmd/openbrain/cli.go cmd/openbrain/cli_test.go cmd/openbrain/main.go`** for accidental complexity.
