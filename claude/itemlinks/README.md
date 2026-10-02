# itemlinks

A Claude Code mod that mirrors the session to a local browser page and hyperlinks
itemized IDs (`F1`, `P2`, `R3`, ...): hover a mention for the item's definition and
its refinements, click to jump to it, copy a reply with the referenced items appended.
The page streams replies as they arrive, follows at reading pace, and can send prompts
back to the session.

## Use

```sh
claude --plugin-dir ~/sandbox/claude/itemlinks
# or, for every session:
export CLAUDE_CODE_PLUGIN_DIRS=~/sandbox/claude/itemlinks
```

The status line shows the page URL (`http://127.0.0.1:<port>/#<token>`); `/itemlinks`
prints it too. Add `?debug` before the `#` for a follow-mode readout.

## How it works

- `hooks/register.ts` (the mod): starts the sidecar, posts each prompt and reply
  (`session.append`), streamed text and thinking (`turn.step`) and the spinner
  (`ui.render`) to it, and submits prompts typed in the page (`$.prompt.submit`, read by
  the model as sent by this plugin).
- `sidecar/server.ts` (Bun): picks a free port and a random token, keeps a per-session
  mirror in `data/`, and on start backfills it from the session's own transcript.
- `sidecar/index.html`: the page. An ID that leads a line or list item is a definition;
  a fresh `X1` without a refinement marker starts a new list of that prefix.

## Security

The token reaches the page only in the URL fragment and is required on every data
route; requests must name `127.0.0.1:<port>` as their host, and prompts need the page's
own origin. Any process running as you can still read the token while the session runs.

Requires Bun at `/opt/homebrew/bin/bun`.
