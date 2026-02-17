# Matrix Go bot (mautrix-go + Ollama)

Matrix bot with command support and optional LLM responses via Ollama.

## Run

```bash
export MATRIX_HOMESERVER="https://<hostname>"
export MATRIX_USER="gobot"
export MATRIX_PASS="<password>"
# optional: room ID or alias to auto-join
export MATRIX_ROOM=""
# optional: file path for persisted sync state
export MATRIX_STATE_FILE=".matrix-bot-state.json"

# optional LLM settings
export OLLAMA_MODEL="mistral-small3.1"
export OLLAMA_URL="http://127.0.0.1:11434"
export OLLAMA_TIMEOUT_SECONDS="90"
# optional: override built-in system prompt
export OLLAMA_SYSTEM_PROMPT=""

# optional routing settings
# ambient mode allows group-room replies without explicit mention/reply
export MATRIX_AMBIENT_MODE="false"
# comma-separated room IDs treated as DM rooms
export MATRIX_DM_ROOMS=""
# optional: room-memory persistence directory (one file per room ID)
export MATRIX_MEMORY_DIR="data/memory"

go run .
```

## Routing Policy

`ShouldRespond` uses first-match-wins:

1. Ignore non-text and self messages.
2. In DM rooms (`MATRIX_DM_ROOMS`): always respond.
3. Commands (`!ping`, `!echo`, `!help`, `!memory`, `!memory clear`) always respond.
4. In group rooms, respond when bot is mentioned, replied to, or thread-targeted.
5. Ignore messages clearly directed to other users.
6. If `MATRIX_AMBIENT_MODE=true`, respond as ambient fallback.

## Context Policy

For LLM calls:

- Current message is always included.
- Recent room history is scored and pruned by relevance and token budget.
- Low-value chatter is de-prioritized.
- Elided high-value items are summarized into a rolling room summary.
- Room memory persists on disk per room at `MATRIX_MEMORY_DIR/<escaped-room-id>.json`.
- Persisted room memory currently stores:
  - `rolling_summary`
  - `durable_memory` (heuristically extracted user constraints/preferences/tasks)

## Notes

- If `MATRIX_ROOM` is empty, the bot listens in rooms it is already in.
- Use an alias like `#room:<hostname>` or a room ID like `!abcdef:<hostname>`.
- The bot persists Matrix sync token in `MATRIX_STATE_FILE`, so restarts do not replay old commands.
- `!memory` prints persisted room summary + durable memory.
- `!memory clear` clears persisted room memory and in-process transcript cache for the current room.
