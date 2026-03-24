# Walkthrough Progress Output Design

**Date:** 2026-03-23

## Goal

Make `scripts/walkthrough.sh` visibly show what it is doing while it runs, without dumping the full generated walkthrough transcript to the terminal.

## Context

`scripts/walkthrough.sh` currently captures the detailed command/response transcript for the generated walkthrough document, but many of the live steps run silently. After the server starts, the script can appear hung while it logs in, creates demo thoughts, and waits for background ingestion to finish.

The generated walkthrough document already serves as the canonical full transcript. The live script only needs enough output to show progress and point people at the generated docs if they want the detailed command/response flow.

## Requirements

1. Print clear, human-readable phase titles while the walkthrough runs.
2. Print progress counters for repeated work that can take noticeable time.
3. Preserve the captured transcript used to render the generated walkthrough documentation.
4. Keep terminal output concise; do not stream full HTTP responses during normal operation.
5. End with a pointer to the generated walkthrough doc for the full transcript.

## Non-Goals

- Replacing the generated walkthrough documentation.
- Printing the full README-style transcript live.
- Adding spinner animations or terminal control sequences.
- Changing the underlying walkthrough data or API behavior.

## Recommended Approach

Add a small set of shell helpers in `scripts/walkthrough.sh` for visible progress output, separate from the existing transcript capture helpers.

### Visible output model

- Use plain log lines, not spinners, so output stays copy/pasteable and easy to test.
- The example phase and progress strings in this spec are illustrative, not an exact text contract; tests should assert the presence of the intended phase/progress concepts without overfitting to punctuation.
- Print phase titles before major phases:
  - starting the local stack
  - pulling Ollama models
  - resetting the database schema
  - provisioning the demo user
  - starting the OpenBrain server
  - logging in
  - creating demo thoughts
  - waiting for background processing
  - querying related thoughts
  - writing the walkthrough doc
- Print progress counters for multi-item operations:
  - `Creating demo thoughts (1/4)` through `(4/4)`
  - `Waiting for background processing (1/4)` through `(4/4)`
- Keep `record_step` and the captured HTTP responses unchanged so doc generation keeps working.
- Visible progress output should be printed directly for the live user experience and must not be appended to `step_responses` or other transcript-capture arrays.

### Script structure changes

- Add one helper for phase messages, e.g. `announce_phase "Starting local stack"`.
- Add one helper for repeated-step progress, e.g. `announce_progress "Creating demo thoughts" 2 4`.
- Call those helpers around existing work rather than rewriting the control flow.
- Update `wait_for_ready_response` callers to announce progress before or after each wait; keep the polling logic unchanged.
- Print a short completion note after the doc is written, pointing users to `docs/walkthrough.demo.md` for the full transcript.

## Alternatives Considered

### 1. Full live transcript

Pros:
- Mirrors the generated walkthrough docs exactly.

Cons:
- Very noisy.
- Harder to read while models pull and HTTP responses scroll by.
- Makes the script worse as an onboarding entry point.

### 2. Minimal heartbeats only

Pros:
- Smallest patch.

Cons:
- Still leaves too much ambiguity about what phase the script is in.
- Less aligned with the structure readers see in the generated docs.

## Error Handling

- Keep existing failures unchanged; the new output should only clarify what phase was running when a failure occurs.
- If a wait times out, the last printed progress line should make it obvious that the script was waiting for background processing.
- If setup fails early (for example the local Postgres runtime conflict), the new phase logging should not obscure the existing actionable error message.

## Testing

Add script tests that assert the walkthrough contains the new progress helper usage and key visible messages, while preserving the existing tests for:
- provisioning with `openbrain user update`
- starting with `go run ./cmd/openbrain start`
- recreating the schema after reset
- detecting a conflicting local Postgres runtime

Run:
- `bash -n scripts/walkthrough.sh`
- focused `go test ./internal/scriptstest -run ...`

## Files

- Modify: `scripts/walkthrough.sh`
- Modify: `internal/scriptstest/provision_demo_data_test.go`

## Success Criteria

1. A user running `scripts/walkthrough.sh` can tell which major phase is currently executing.
2. The previously silent thought creation and readiness wait now show progress counters.
3. The generated walkthrough document still renders from the captured transcript without format regressions.
4. Script-focused tests cover the visible progress contract.
