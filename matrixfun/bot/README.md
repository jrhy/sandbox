# Matrix Go bot (mautrix-go)

Simple bot that replies to `!ping`, `!echo <text>`, and `!help`.

## Run

```bash
export MATRIX_HOMESERVER="https://<hostname>"
export MATRIX_USER="gobot"
export MATRIX_PASS="<password>"
# optional: room ID or alias to auto-join
export MATRIX_ROOM=""

go run .
```

## Notes

- If `MATRIX_ROOM` is empty, the bot listens in rooms it is already in.
- Use an alias like `#room:<hostname>` or a room ID like `!abcdef:<hostname>`.
