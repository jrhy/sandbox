from __future__ import annotations

import argparse
from contextlib import nullcontext
from dataclasses import asdict, dataclass
import json
from pathlib import Path
import tempfile
from typing import Any

from .bot import BotMessage, MatrixMemoryBot
from .config import (
    DEFAULT_KUZU_PATH,
    DEFAULT_MEM0_COLLECTION_NAME,
    DEFAULT_OLLAMA_CHAT_MODEL,
    DEFAULT_OLLAMA_EMBEDDING_DIMS,
    DEFAULT_OLLAMA_EMBED_MODEL,
)
from .memory import MemoryClient


DEFAULT_ROOM_ID = "!matrixmem:local"
DEFAULT_USER_ID = "@alice:local"
DEFAULT_STATEMENT_EVENT_ID = "$statement"
DEFAULT_QUERY_EVENT_ID = "$query"
DEFAULT_STATEMENT_TIMESTAMP = "2026-04-12T00:00:00+00:00"
DEFAULT_QUERY_TIMESTAMP = "2026-04-12T00:00:01+00:00"
DEFAULT_STATEMENT_TEXT = "Need to buy solder wick"
DEFAULT_QUERY_TEXT = "What should I buy?"


@dataclass(frozen=True, slots=True)
class ReplyRecord:
    room_id: str
    event_id: str
    text: str


class RecordingReplySink:
    def __init__(self) -> None:
        self.calls: list[ReplyRecord] = []

    def __call__(self, *, room_id: str, event_id: str, text: str) -> None:
        self.calls.append(
            ReplyRecord(
                room_id=room_id,
                event_id=event_id,
                text=text,
            )
        )


class FakeMemoryClient:
    def __init__(self) -> None:
        self.add_calls: list[dict[str, Any]] = []
        self.query_calls: list[dict[str, Any]] = []
        self.stored_memories: list[dict[str, Any]] = []

    def add_message(
        self,
        *,
        text: str,
        user_id: str,
        room_id: str,
        event_id: str,
        sender_id: str,
        timestamp: str,
        handling_mode: str,
        metadata: Any = None,
    ) -> dict[str, Any]:
        del metadata
        call = {
            "text": text,
            "user_id": user_id,
            "room_id": room_id,
            "event_id": event_id,
            "sender_id": sender_id,
            "timestamp": timestamp,
            "handling_mode": handling_mode,
        }
        self.add_calls.append(call)
        self.stored_memories.append(call)
        return call

    def answer_query(
        self,
        query: str,
        *,
        user_id: str,
        room_id: str,
        metadata: Any = None,
    ) -> dict[str, Any]:
        del metadata
        call = {
            "query": query,
            "user_id": user_id,
            "room_id": room_id,
        }
        self.query_calls.append(call)
        memories = [
            {
                "memory": item["text"],
                "score": 1.0,
            }
            for item in self.stored_memories
            if item["room_id"] == room_id and item["user_id"] == user_id
        ]
        answer = "I couldn't find anything relevant yet."
        if memories:
            answer = f"You said to buy: {memories[0]['memory']}."
        return {
            "answer": answer,
            "results": memories,
            "relations": [],
        }

    def snapshot(self) -> dict[str, Any]:
        return {
            "add_calls": list(self.add_calls),
            "query_calls": list(self.query_calls),
            "stored_memories": list(self.stored_memories),
        }


def run_harness(
    *,
    mode: str,
    archive_path: str | Path | None = None,
    statement_text: str = DEFAULT_STATEMENT_TEXT,
    query_text: str = DEFAULT_QUERY_TEXT,
    room_id: str = DEFAULT_ROOM_ID,
    user_id: str = DEFAULT_USER_ID,
    skip_ingest: bool = False,
) -> dict[str, Any]:
    cleanup_context = nullcontext(None)
    if archive_path is None and mode == "mock":
        cleanup_context = tempfile.TemporaryDirectory(prefix="matrixmem-harness-")

    with cleanup_context as tempdir:
        harness_archive_path = _resolve_archive_path(
            mode=mode,
            archive_path=archive_path,
            tempdir=tempdir,
        )
        reply_sink = RecordingReplySink()
        memory_client = _build_memory_client(mode)

        bot = MatrixMemoryBot(
            room_id=room_id,
            bot_user_id="@bot:local",
            archive_path=harness_archive_path,
            memory_client=memory_client,
            reply_sink=reply_sink,
        )

        ingest_result = None
        if not skip_ingest:
            ingest_result = bot.handle_message(
                BotMessage(
                    room_id=room_id,
                    event_id=DEFAULT_STATEMENT_EVENT_ID,
                    sender_id=user_id,
                    timestamp=DEFAULT_STATEMENT_TIMESTAMP,
                    text=statement_text,
                )
            )

        query_result = bot.handle_message(
            BotMessage(
                room_id=room_id,
                event_id=DEFAULT_QUERY_EVENT_ID,
                sender_id=user_id,
                timestamp=DEFAULT_QUERY_TIMESTAMP,
                text=query_text,
            )
        )

        return {
            "mode": mode,
            "archive_path": str(harness_archive_path),
            "archive_records": _read_archive_records(harness_archive_path),
            "ingest_result": ingest_result,
            "query_result": query_result["memory_result"],
            "reply": asdict(reply_sink.calls[-1]) if reply_sink.calls else None,
            "memory_client": _memory_snapshot(memory_client),
        }


def main(argv: list[str] | None = None) -> int:
    args = _build_parser().parse_args(argv)
    result = run_harness(
        mode=args.mode,
        archive_path=args.archive_path,
        statement_text=args.statement,
        query_text=args.query,
        room_id=args.room_id,
        user_id=args.user_id,
        skip_ingest=args.skip_ingest,
    )
    print(json.dumps(result, indent=2, sort_keys=True))
    return 0


def _build_memory_client(mode: str) -> Any:
    if mode == "mock":
        return FakeMemoryClient()
    if mode == "real":
        return MemoryClient(
            qdrant_url=_require_env("QDRANT_URL"),
            kuzu_path=_env_or_default("KUZU_PATH", DEFAULT_KUZU_PATH),
            ollama_base_url=_require_env("OLLAMA_BASE_URL"),
            collection_name=_env_or_default("MEM0_COLLECTION_NAME", DEFAULT_MEM0_COLLECTION_NAME),
            chat_model=_env_or_default("OLLAMA_CHAT_MODEL", DEFAULT_OLLAMA_CHAT_MODEL),
            embed_model=_env_or_default("OLLAMA_EMBED_MODEL", DEFAULT_OLLAMA_EMBED_MODEL),
            embedding_dims=int(
                _env_or_default(
                    "OLLAMA_EMBEDDING_DIMS",
                    str(DEFAULT_OLLAMA_EMBEDDING_DIMS),
                )
            ),
        )
    raise ValueError(f"Unsupported harness mode: {mode}")


def _resolve_archive_path(
    *,
    mode: str,
    archive_path: str | Path | None,
    tempdir: str | None = None,
) -> Path:
    if archive_path is not None:
        return Path(archive_path)
    if mode == "real":
        return Path("/data/archive/harness-events.jsonl")
    if tempdir is None:
        tempdir = tempfile.mkdtemp(prefix="matrixmem-harness-")
    return Path(tempdir) / "events.jsonl"


def _read_archive_records(archive_path: Path) -> list[dict[str, Any]]:
    if not archive_path.exists():
        return []
    return [
        json.loads(line)
        for line in archive_path.read_text(encoding="utf-8").splitlines()
        if line.strip()
    ]


def _memory_snapshot(memory_client: Any) -> dict[str, Any] | None:
    if hasattr(memory_client, "snapshot"):
        return memory_client.snapshot()
    return None


def _build_parser() -> argparse.ArgumentParser:
    parser = argparse.ArgumentParser(description="Run the matrixmem local harness")
    parser.add_argument("--mode", choices=("mock", "real"), default="mock")
    parser.add_argument("--archive-path", default=None)
    parser.add_argument("--statement", default=DEFAULT_STATEMENT_TEXT)
    parser.add_argument("--query", default=DEFAULT_QUERY_TEXT)
    parser.add_argument("--room-id", default=DEFAULT_ROOM_ID)
    parser.add_argument("--user-id", default=DEFAULT_USER_ID)
    parser.add_argument("--skip-ingest", action="store_true")
    return parser


def _require_env(name: str) -> str:
    from os import environ

    value = environ.get(name, "").strip()
    if not value:
        raise ValueError(f"Missing required environment variable for real harness mode: {name}")
    return value


def _env_or_default(name: str, default: str) -> str:
    from os import environ

    value = environ.get(name, "").strip()
    if value:
        return value
    return default


if __name__ == "__main__":
    raise SystemExit(main())
