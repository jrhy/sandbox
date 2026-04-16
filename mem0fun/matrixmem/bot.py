from __future__ import annotations

from dataclasses import dataclass
from pathlib import Path
from typing import Any, Callable, Mapping

from .archive import append_event
from .classify import classify_message


@dataclass(frozen=True, slots=True)
class BotMessage:
    room_id: str
    event_id: str
    sender_id: str
    timestamp: str
    text: str
    message_type: str = "m.text"


class MatrixMemoryBot:
    def __init__(
        self,
        *,
        room_id: str,
        bot_user_id: str,
        archive_path: str | Path,
        memory_client: Any,
        reply_sink: Callable[..., None] | None = None,
        archive_writer: Callable[..., Any] = append_event,
        classifier: Callable[..., str] = classify_message,
    ) -> None:
        self.room_id = room_id
        self.bot_user_id = bot_user_id
        self.archive_path = Path(archive_path)
        self.memory_client = memory_client
        self.reply_sink = reply_sink
        self.archive_writer = archive_writer
        self.classifier = classifier

    def subscribe(self, gateway: Any) -> Any:
        return gateway.subscribe_room(self.room_id, self.handle_event)

    def handle_event(self, event: Any) -> dict[str, Any]:
        message = self._normalize_event(event)
        if message.message_type != "m.text":
            return {"handling_mode": "ignore"}
        return self.handle_message(message)

    def handle_message(self, message: BotMessage) -> dict[str, Any]:
        if message.room_id != self.room_id:
            return {"handling_mode": "ignore"}

        handling_mode = self.classifier(
            text=message.text,
            sender_id=message.sender_id,
            bot_user_id=self.bot_user_id,
        )
        if handling_mode == "ignore":
            return {"handling_mode": "ignore"}

        archive_record = self.archive_writer(
            self.archive_path,
            room_id=message.room_id,
            event_id=message.event_id,
            timestamp=message.timestamp,
            text=message.text,
            sender_id=message.sender_id,
            handling_mode=handling_mode,
        )

        if handling_mode == "ingest":
            memory_result = self.memory_client.add_message(
                text=message.text,
                user_id=message.sender_id,
                room_id=message.room_id,
                event_id=message.event_id,
                sender_id=message.sender_id,
                timestamp=message.timestamp,
                handling_mode=handling_mode,
            )
            return {
                "handling_mode": handling_mode,
                "archive_record": archive_record,
                "memory_result": memory_result,
            }

        if handling_mode == "query":
            memory_result = self.memory_client.answer_query(
                message.text,
                user_id=message.sender_id,
                room_id=message.room_id,
            )
            self._reply(message.room_id, message.event_id, memory_result["answer"])
            return {
                "handling_mode": handling_mode,
                "archive_record": archive_record,
                "memory_result": memory_result,
            }

        raise ValueError(f"Unknown handling mode: {handling_mode}")

    def _reply(self, room_id: str, event_id: str, text: str) -> None:
        if self.reply_sink is None:
            return
        self.reply_sink(room_id=room_id, event_id=event_id, text=text)

    def _normalize_event(self, event: Any) -> BotMessage:
        if isinstance(event, BotMessage):
            return event

        if isinstance(event, Mapping):
            data = event
        else:
            data = {
                "room_id": getattr(event, "room_id"),
                "event_id": getattr(event, "event_id"),
                "sender_id": getattr(event, "sender_id"),
                "timestamp": getattr(event, "timestamp", ""),
                "text": getattr(event, "text", getattr(event, "body", "")),
                "message_type": getattr(
                    event,
                    "message_type",
                    getattr(event, "msgtype", "m.text"),
                ),
            }

        content = data.get("content")
        content_map = content if isinstance(content, Mapping) else {}
        text = data.get("text", content_map.get("body", ""))
        message_type = data.get(
            "message_type",
            data.get("msgtype", content_map.get("msgtype", "m.text")),
        )

        return BotMessage(
            room_id=str(data["room_id"]),
            event_id=str(data["event_id"]),
            sender_id=str(data["sender_id"]),
            timestamp=str(data["timestamp"]),
            text=str(text),
            message_type=str(message_type),
        )
