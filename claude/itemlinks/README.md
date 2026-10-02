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

The working convention this mod serves: Add the following to CLAUDE.md so items are labeled and referenced:

> itemize points with identifiers, such as `F1, F2, ...` for Findings, `P1, P2, ...` for Premises, `R1, R2, ...` for Requirements, and later in the session refer back to them by identifier instead of repeating. When one needs refining, tag the refinement/clarification/extension with the same identifier, so its evolution is easy to trace.

In the terminal each back-reference costs a scrollback search; here it is a hover.

The page infers everything from the text; nothing is annotated by hand.

- **Definition:** an ID that leads a line or list item, followed by `:`, `.`, `)` or a
  dash. Bold is fine. All of these define `F3`: `**F3:** ...`, `- F3. ...`,
  `3. F3) ...`, `F3 - ...`. An ID is one to three capitals plus a number, with an
  optional sub-number (`F3`, `R12`, `P1.2`, `FM2`). Pick one prefix per category and
  keep it for the session.
- **Reference:** any later mention of a known ID, in a prompt or a reply, becomes a
  link. A mention before the definition stays plain text.
- **Refinement:** define the ID again with "refined", "clarified", "extended",
  "updated", "revised" or "amended" in the line, e.g. `**F3 (refined):** ...`. The
  hover card stacks the original and each refinement, labeled by turn; clicking jumps
  to the latest version. A recap that repeats an item verbatim is not shown as a
  refinement.
- **Reusing a number:** a fresh `X1` with no refinement wording starts a new list of
  prefix X; later mentions resolve to the new list.
- **Copy w/ refs:** each row's button copies its text with the definitions of every ID
  it mentions appended as quotes, the shape to paste where readers lack the session.

## Reading the page

- **Follow:** a new reply glides into view, then scrolls at reading pace. Scrolling
  yourself pauses it; the bottom-right button toggles "⏸ reading pace" / "▶ follow",
  and reaching the bottom resumes. Your own prompts and collapsed tool rows are
  skimmed.
- **Streaming:** a reply shows as a growing draft until the stored row replaces it;
  thinking appears collapsed.
- **Tool rows:** name, summary fields and outcome; output streams live, cut to 40
  lines, never stored.
- **Prompt box:** text sent from the page is submitted between turns, marked as coming
  from the plugin. Enter or Cmd/Ctrl+Enter sends.
- **Stale token:** the token rotates on every mod reload and the page says so. Run
  `/itemlinks` and open the new URL.

## How it works

- `hooks/register.ts` (the mod): starts the sidecar, posts each prompt and reply
  (`session.append`), streamed text and thinking (`turn.step`), the spinner
  (`ui.render`) and each tool call (`tool.call`) to it, and submits prompts typed in the page (`$.prompt.submit`, read by
  the model as sent by this plugin).
- `sidecar/server.ts` (Bun): picks a free port and a random token, keeps a per-session
  mirror in `data/`, and on start backfills it from the session's own transcript.
  Tool calls are kept as name, a one-line summary and outcome; their output reaches
  the page live but is never written to disk.
- `sidecar/index.html`: the page. An ID that leads a line or list item is a definition;
  a fresh `X1` without a refinement marker starts a new list of that prefix.

## Security

The token reaches the page only in the URL fragment and is required on every data
route; requests must name `127.0.0.1:<port>` as their host, and prompts need the page's
own origin. Any process running as you can still read the token while the session runs.

Requires Bun at `/opt/homebrew/bin/bun`.
