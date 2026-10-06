---
name: Sandboxed Worktree Pod
description: A coder and a reviewer that work on isolated git worktrees of one repo under an OS sandbox, with a human as the only path to origin.
schema: agentcompanies/v1
slug: sandboxed-worktree-pod
category: software-development
key: jeffr/local/software-development/sandboxed-worktree-pod
includes:
  - agents/coder/AGENTS.md
  - agents/reviewer/AGENTS.md
  - agents/verifier/AGENTS.md
  - agents/simplifier/AGENTS.md
  - agents/observer/AGENTS.md
defaultInstall: false
tags:
  - engineering
  - code-review
  - sandbox
---

# Sandboxed Worktree Pod

Two roles, no manager agent. Routing is fixed by the operator rather than decided by an
LLM: the operator files issues, assigns them, and is the only party that can push to origin
or open a pull request. Every agent runs under `paperclip-sb-claude`, which confines it to
its worktree, its own config dir, the LLM proxy and the Paperclip API. These files describe
the roles; the sandbox enforces them.

## Roles

- `coder` — works in exactly one isolated worktree (`bin/worktree create <name> --isolated`,
  branch `agent/<name>`). Commits there, never pushes. One in-flight issue per worktree.
- `reviewer` — read-only. Adversarial: correctness, regressions, claims the code does not
  back. Reproduces before it reports. Cannot commit anywhere.
- `verifier` — read-only. Re-runs every check a coder claimed, independently, and says per
  claim whether it observed the same thing. A coder's `done` is not accepted until the
  verifier does. Exists because every before/after check so far was self-reported.
- `simplifier` — read-only. Asks, once a change is correct: is it the simplest solution that
  is appropriately general? Proposals carry a concrete diff and a cost line, or they are not
  proposals. "No change recommended" is a complete result.

Not defined yet, and the trigger for each:

- `lead` — would create child issues within one project (stage 3 below). Written when a
  human-routed cycle has run cleanly at least three times and the routing decisions were
  mechanical every time.
- `releaser` — the only agent allowed to push or open PRs, through a GitHub connection with a
  scoped token (stage 4). Needs Paperclip's MCP access governance set up first.

## The loop

1. Operator creates a worktree, binds a `coder` (`paperclip-agent create|bind ... --cwd`),
   files one issue, assigns it.
2. `coder` works it to `done` with a per-finding report, or to `blocked` naming who can
   unblock it. It does not open issues for other agents.
3. Operator imports the branch (`paperclip-agent import <worktree>`) and files a `verifier`
   issue naming the worktree and the coder's report. `return` goes back to the coder as a
   new issue quoting the failed claims; `accept` continues.
4. Operator files a `reviewer` issue. `request changes` becomes coder issues, one worktree
   per cluster, back to step 1; conflicting fixes are re-run on the new base, not rebased.
   `approve` continues.
5. Operator files a `simplifier` issue. Proposals marked `worth it` become coder issues,
   back to step 1. `no change recommended` ends the loop.
6. Operator pushes and owns the PR.

Stopping rule, so the loop cannot become an open-ended spend: the loop ends at the first
round where the reviewer approves and the simplifier recommends no change, or after four
rounds, or when the company budget cap is hit, whichever comes first. A round that ends on
the cap or the count is reported as such, not as approved.

## Stages of agent autonomy

Each stage adds exactly one capability. Everything not listed stays with the operator.

| stage | agents may additionally | status |
|---|---|---|
| 0 | comment, set their own issue's status, commit in their own worktree | done |
| 1 | `verifier` and `simplifier` roles receive issues (routing still human) | current |
| 2 | reviewer/verifier set `in_review` and raise a Paperclip interaction (question or confirmation) instead of writing "operator must..." in prose | next |
| 3 | one `lead` role creates child issues within one project, under a round cap and budget | not yet |
| 4 | `releaser` pushes and opens PRs through a scoped GitHub token; the operator approves via `request_board_approval` | not yet |

Stage 3 is the only one that lets an agent spend money on another agent's behalf. The
default Paperclip execution contract's "create child issues directly" rule is deliberately
absent from every AGENTS.md here until that stage.

## Standing rules (repeated in each AGENTS.md)

- Start actionable work in the same heartbeat; do not stop at a plan unless the issue asks
  for one.
- Final disposition: `done` only when complete and verified by your own checks; `blocked`
  only with a named unblock owner; never leave an issue `in_progress` on exit. Comments and
  "remaining" bullets are evidence, not a reason to leave an issue open.
- Respect budget, pause/cancel and company boundaries.

- Reproduce before reporting; say what was reproduced.
- One commit per finding; conventional-commit prefix; plain message, no trailers, no
  attribution lines.
- Never push, never open PRs, never widen your own grants.
- Throwaway repos go under `$TMPDIR`; leave nothing behind in the worktree.
- When blocked, stop and name the owner of the unblock rather than working around a denial.
