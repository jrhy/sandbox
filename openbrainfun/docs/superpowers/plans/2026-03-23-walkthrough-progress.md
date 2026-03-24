# Walkthrough Progress Output Implementation Plan

> **For agentic workers:** REQUIRED: Use superpowers:subagent-driven-development (if subagents available) or superpowers:executing-plans to implement this plan. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Add concise live phase and progress output to `scripts/walkthrough.sh` so users can tell what the walkthrough is doing, without changing the captured transcript that renders the generated walkthrough docs.

**Architecture:** Keep the existing transcript capture path intact, and add a separate thin layer of shell helpers for live progress output. Update the walkthrough’s existing setup, thought-creation, readiness-wait, and completion phases to call those helpers, and lock the behavior in with focused script tests that assert concepts instead of brittle punctuation.

**Tech Stack:** Bash, Go `testing`, existing script text assertions in `internal/scriptstest`

---

## File Structure

- Modify: `scripts/walkthrough.sh:39-103,170-247,278+`
  - Responsibility: define visible progress helpers, announce major phases, add progress counters for repeated work, and print a short completion pointer to the generated walkthrough doc.
- Modify: `internal/scriptstest/provision_demo_data_test.go:61-108`
  - Responsibility: assert the walkthrough script contains the visible progress contract while preserving the existing provisioning, schema reset, and conflict-detection expectations.

## Chunk 1: Add and verify live walkthrough progress output

### Task 1: Lock the visible phase-output contract in tests

**Files:**
- Modify: `internal/scriptstest/provision_demo_data_test.go:61-108`
- Test: `internal/scriptstest/provision_demo_data_test.go`

- [ ] **Step 1: Write the failing test**

Add a focused test such as `TestWalkthroughScriptAnnouncesLivePhases` that reads `scripts/walkthrough.sh` and asserts the script text includes the live-output concepts below without overfitting to exact punctuation:

```go
for _, want := range []string{
    "announce_phase",
    "Starting local stack",
    "Pulling Ollama models",
    "Resetting demo schema",
    "Provisioning demo user",
    "Starting OpenBrain server",
    "Logging in",
    "Creating demo thoughts",
    "Waiting for background processing",
    "Querying related thoughts",
    "Writing walkthrough doc",
    "Full transcript:",
    "docs/walkthrough.demo.md",
} {
    if !strings.Contains(text, want) {
        t.Fatalf("missing %q in walkthrough script:\n%s", want, text)
    }
}
```

- [ ] **Step 2: Run test to verify it fails**

Run:

```bash
cd openbrainfun
GOCACHE=/tmp/go-build-cache go test ./internal/scriptstest -run TestWalkthroughScriptAnnouncesLivePhases -v
```

Expected: FAIL because `scripts/walkthrough.sh` does not yet define the live-output helper or phase messages.

- [ ] **Step 3: Write minimal implementation**

Add a small visible-output helper layer near the existing transcript helpers in `scripts/walkthrough.sh`, for example:

```bash
announce_phase() {
  printf '\n==> %s\n' "$1"
}
```

Then call it before the major phases already present in the script:
- container startup / stack wait
- Ollama model pull
- schema reset / migration
- demo user provisioning
- server startup
- login
- creating demo thoughts
- waiting for background processing
- related-thought queries
- walkthrough doc rendering / completion

Add an explicit final completion line that points users to the generated walkthrough doc, for example `Full transcript: docs/walkthrough.demo.md`.

Do not append these visible messages to `step_responses` or any transcript array.

- [ ] **Step 4: Run test to verify it passes**

Run:

```bash
cd openbrainfun
GOCACHE=/tmp/go-build-cache go test ./internal/scriptstest -run TestWalkthroughScriptAnnouncesLivePhases -v
```

Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add internal/scriptstest/provision_demo_data_test.go scripts/walkthrough.sh
git commit -m "openbrainfun/scripts: announce walkthrough phases"
```

### Task 2: Add progress counters for the repeated work that currently feels hung

**Files:**
- Modify: `internal/scriptstest/provision_demo_data_test.go`
- Modify: `scripts/walkthrough.sh:94-103,208-237`
- Test: `internal/scriptstest/provision_demo_data_test.go`

- [ ] **Step 1: Extend the test with a failing progress assertion**

Add a second focused test such as `TestWalkthroughScriptAnnouncesCreationAndReadyProgress` that asserts the script text includes progress helper usage for the two repeated loops:

```go
for _, want := range []string{
    `announce_progress "Creating demo thoughts" 1 4`,
    `announce_progress "Creating demo thoughts" 4 4`,
    `announce_progress "Waiting for background processing" 1 4`,
    `announce_progress "Waiting for background processing" 4 4`,
} {
    if !strings.Contains(text, want) {
        t.Fatalf("missing %q in walkthrough script:\n%s", want, text)
    }
}
```

- [ ] **Step 2: Run test to verify it fails**

Run:

```bash
cd openbrainfun
GOCACHE=/tmp/go-build-cache go test ./internal/scriptstest -run TestWalkthroughScriptAnnouncesCreationAndReadyProgress -v
```

Expected: FAIL because the script still creates thoughts and waits for readiness silently.

- [ ] **Step 3: Write minimal implementation**

Add a small progress helper, for example:

```bash
announce_progress() {
  printf '  - %s (%s/%s)\n' "$1" "$2" "$3"
}
```

Use it around the existing repeated operations without changing the captured HTTP transcript:
- keep a phase title before the create-thought section and then add four explicit `announce_progress "Creating demo thoughts" N 4` call sites around the existing `create_thought` calls
- keep a phase title before the readiness-wait section and then add four explicit `announce_progress "Waiting for background processing" N 4` call sites around the existing `wait_for_ready_response` calls

Keep `record_step` and the `wait_for_ready_response` polling logic intact.

- [ ] **Step 4: Run test to verify it passes**

Run:

```bash
cd openbrainfun
GOCACHE=/tmp/go-build-cache go test ./internal/scriptstest -run 'TestWalkthroughScriptAnnounces(LivePhases|CreationAndReadyProgress)' -v
```

Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add internal/scriptstest/provision_demo_data_test.go scripts/walkthrough.sh
git commit -m "openbrainfun/scripts: show walkthrough progress"
```

### Task 3: Verify the final script behavior and guardrails

**Files:**
- Modify: `internal/scriptstest/provision_demo_data_test.go` (only if verification exposes gaps)
- Modify: `scripts/walkthrough.sh` (only if verification exposes gaps)
- Test: `scripts/walkthrough.sh`, `internal/scriptstest/provision_demo_data_test.go`

- [ ] **Step 1: Check shell syntax**

Run:

```bash
cd openbrainfun
bash -n scripts/walkthrough.sh
```

Expected: no output, zero exit status.

- [ ] **Step 2: Run the focused walkthrough script tests**

Run:

```bash
cd openbrainfun
GOCACHE=/tmp/go-build-cache go test ./internal/scriptstest -run 'TestWalkthroughScript(UsesOpenbrainCLIForProvisioningAndStart|RecreatesSchemaAfterReset|DetectsConflictingMacOSContainerRuntime|AnnouncesLivePhases|AnnouncesCreationAndReadyProgress)' -v
```

Expected: PASS for all listed tests.

- [ ] **Step 3: Run the full Go test suite**

Run:

```bash
cd openbrainfun
GOCACHE=/tmp/go-build-cache go test ./...
```

Expected: PASS for the full `openbrainfun` suite.

- [ ] **Step 4: Commit any verification-driven fixes**

If verification required final polish:

```bash
git add internal/scriptstest/provision_demo_data_test.go scripts/walkthrough.sh
git commit -m "openbrainfun/scripts: polish walkthrough progress output"
```

If no further changes were needed, explicitly note that no extra commit is required.
