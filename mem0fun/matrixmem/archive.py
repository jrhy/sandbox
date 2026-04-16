from __future__ import annotations

import fcntl
import os
from pathlib import Path
import json
from typing import Any


def append_event(
    archive_path: str | Path,
    *,
    room_id: str,
    event_id: str,
    timestamp: str,
    text: str,
    sender_id: str,
    handling_mode: str,
) -> dict[str, Any]:
    path = Path(archive_path)
    path.parent.mkdir(parents=True, exist_ok=True)

    record = {
        "room_id": room_id,
        "event_id": event_id,
        "timestamp": timestamp,
        "text": text,
        "sender_id": sender_id,
        "handling_mode": handling_mode,
    }

    payload = (json.dumps(record, sort_keys=True, separators=(",", ":")) + "\n").encode(
        "utf-8"
    )

    fd = os.open(path, os.O_WRONLY | os.O_CREAT | os.O_APPEND, 0o644)
    try:
        fcntl.flock(fd, fcntl.LOCK_EX)
        _write_all(fd, payload)
        os.fsync(fd)
    finally:
        fcntl.flock(fd, fcntl.LOCK_UN)
        os.close(fd)

    return record


def _write_all(fd: int, payload: bytes) -> None:
    view = memoryview(payload)
    while view:
        written = os.write(fd, view)
        view = view[written:]
