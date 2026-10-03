# paperclipfun

Run [Paperclip](https://github.com/paperclipai/paperclip) locally with every agent
confined by `sb exec` (macOS seatbelt): one isolated git worktree, one LLM proxy host,
the Paperclip API on localhost, and nothing else. Reviewer agents get a read-only view of
the worktree they review and cannot commit anywhere.

Three scripts, meant to be symlinked into `~/bin`:

| script | role |
|---|---|
| `paperclip-proxy` | Starts Paperclip (`pnpm dev` in the source checkout). Mints the proxy token, pins `ANTHROPIC_BASE_URL` to the proxy, strips every other credential from the environment, turns off telemetry, supplies the agent JWT secret. |
| `paperclip-sb-claude` | What Paperclip runs per agent turn instead of `claude`. Builds the `sb exec` grant list from the agent's env and cwd, then execs `claude` inside it. |
| `paperclip-agent` | `bootstrap` (prereq checks, create company), `create` (an agent: config dir, persistent home, Paperclip record), `import` (copy an agent worktree's commits into the main repo, re-hashed). |

Settings come from `~/.config/paperclipfun/env`; see `env.example`. Two are required:
the proxy host and the main repo's `.git` dir.

## Setup

1. Prerequisites: Node 24, pnpm 9, Rust (`brew install rustup && rustup default stable`,
   PATH += `/opt/homebrew/opt/rustup/bin`), the `claude` CLI run once, `jq`, and `sb` built
   from `../cmd/sb` (needs `--allow-read/--allow-write/--home`).
2. `git clone https://github.com/paperclipai/paperclip ~/src/paperclip && cd ~/src/paperclip && pnpm install`
3. `cp env.example ~/.config/paperclipfun/env` and edit.
4. `paperclip-proxy` in a terminal. First start builds the native runner (a few minutes).
5. `paperclip-agent bootstrap "My Org"` creates the company. Skip Paperclip's onboarding
   wizard: its API-key step calls `api.anthropic.com` directly and rejects proxy tokens.
   Opening `http://localhost:3100` afterwards lands on the board, not the wizard.

## Working with agents

Agent worktrees come from the monorepo's `bin/worktree create <name> --isolated`, which
checks out branch `agent/<name>` with no symlinks into the main checkout. Then:

    paperclip-agent create coder-x   --role coder    --model claude-sonnet-5-5 --cwd ~/worktrees/x
    paperclip-agent create reviewer-x --role reviewer --model claude-opus-5-5  --review ~/worktrees/x

File issues through the Paperclip UI or API and assign them. One in-flight issue per
worktree: two agents committing in the same worktree pick up each other's edits.

To review an agent's work on the host, first `paperclip-agent import ~/worktrees/x`, then
`git log agent/x` from the main checkout. Never run git inside an agent worktree on the
host: the worktree's `.git` file and admin dir are agent-writable.

## What the sandbox grants a coder

- read/write: the worktree, `CLAUDE_CONFIG_DIR`, Paperclip's per-run scratch dir
- read: the main repo's `.git`, the agent's instructions and prompt cache under
  `~/.paperclip`, the Go toolchain and module cache, `/opt/homebrew`
- git: writes to its own admin dir, `refs/heads/agent/` and its reflogs; new objects go to
  a private store (`.git/worktrees/<name>/objects`), never the shared one
- network: HTTP(S) to the LLM proxy host only; TCP to the Paperclip API port on localhost
- a persistent fake `HOME` per agent (tool caches survive between runs; pre-warm host-built
  tools with `HOME=<agent config dir>/home <tool>` once)

Everything else under `~` is unreadable, including `~/.aws`, `~/.ssh`, `~/.claude`.

## Why the private object store is never an alternate

Git trusts loose objects by file name. An agent that could register its store as an
alternate of the main repo could plant a forged object under the id of an unfetched
upstream commit, and the host's next `git fetch` would accept it as already present.
`paperclip-agent import` copies objects through `index-pack --strict` instead.
