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
- `reviewer` — read-only. Gets a worktree path to review and a scratch workspace of its own.
  Reproduces before it reports. Cannot commit anywhere.

Not defined yet, and why:

- `verifier` — would re-run the checks a coder claimed, independently, in a fresh worktree.
  Needs the same grants as a coder minus git write. Not written until a coder's self-report
  has been wrong once in a way a verifier would have caught.
- `releaser` — the only agent allowed to push or open PRs, through a GitHub connection with a
  scoped token. Needs Paperclip's MCP access governance set up first. Until then the operator
  is the releaser.

## Handoffs

1. Operator creates the worktree, binds an agent to it (`paperclip-agent create ... --cwd`),
   files one issue, assigns it.
2. `coder` works the issue to `done` with a per-finding report, or to `blocked` naming who
   can unblock it. It does not open review issues itself.
3. Operator imports the branch (`paperclip-agent import <worktree>`), reads it from the main
   checkout, and files a review issue for `reviewer` when the change warrants one.
4. `reviewer` posts numbered findings and a verdict, marks `done`.
5. Operator files fix issues from the reviewer's own text, one worktree per cluster, back to
   step 1. Conflicting fixes are re-run on the new base rather than rebased.
6. Operator pushes and owns the PR.

Steps 3 and 5 are deliberately human: they are where this pod's judgement is checked. Making
either one an agent decision is the next experiment, not the default.

## Standing rules (repeated in each AGENTS.md)

- Reproduce before reporting; say what was reproduced.
- One commit per finding; conventional-commit prefix; plain message, no trailers, no
  attribution lines.
- Never push, never open PRs, never widen your own grants.
- Throwaway repos go under `$TMPDIR`; leave nothing behind in the worktree.
- When blocked, stop and name the owner of the unblock rather than working around a denial.
