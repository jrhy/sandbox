from __future__ import annotations

import asyncio
from pathlib import Path
from typing import Any, Callable

from .bot import MatrixMemoryBot
from .config import Config, load_config
from .memory import MemoryClient


class NioMatrixGateway:
    def __init__(
        self,
        *,
        homeserver_url: str,
        access_token: str,
        sync_timeout_ms: int = 30_000,
        client: Any | None = None,
        room_message_text_type: Any | None = None,
    ) -> None:
        self.homeserver_url = homeserver_url
        self.access_token = access_token
        self.sync_timeout_ms = sync_timeout_ms
        self._room_id = ""
        self._handler: Callable[[Any], Any] | None = None
        self._bot: Any = None
        self._loop: asyncio.AbstractEventLoop | None = None
        self._room_message_text_type = room_message_text_type

        if client is None:
            from nio import AsyncClient

            client = AsyncClient(homeserver_url)

        self._client = client
        self._client.access_token = access_token

    def subscribe_room(self, room_id: str, handler: Callable[[Any], Any]) -> str:
        self._room_id = room_id
        self._handler = handler
        self._bot = getattr(handler, "__self__", None)
        return room_id

    def run_forever(self) -> None:
        asyncio.run(self._run_forever())

    def reply(self, *, room_id: str, event_id: str, text: str) -> None:
        if self._loop is None:
            raise RuntimeError("Matrix gateway loop is not running")
        reply_future = asyncio.run_coroutine_threadsafe(
            self._send_text(room_id=room_id, event_id=event_id, text=text),
            self._loop,
        )
        reply_future.result()

    async def _run_forever(self) -> None:
        if not self._room_id or self._handler is None:
            raise RuntimeError("No room subscription configured")

        room_message_text_type = self._room_message_text_type
        if room_message_text_type is None:
            from nio import RoomMessageText

            room_message_text_type = RoomMessageText

        self._loop = asyncio.get_running_loop()
        await self._bind_bot_identity()
        self._client.add_event_callback(self._on_room_message, room_message_text_type)
        await self._client.sync_forever(
            timeout=self.sync_timeout_ms,
            sync_filter={
                "room": {
                    "rooms": [self._room_id],
                    "timeline": {
                        "types": ["m.room.message"],
                    },
                }
            },
        )

    async def _bind_bot_identity(self) -> None:
        response = await self._client.whoami()
        user_id = getattr(response, "user_id", "")
        if not user_id:
            raise RuntimeError("Unable to resolve Matrix user id from access token")

        self._client.user = user_id
        if self._bot is not None:
            self._bot.bot_user_id = user_id

    async def _on_room_message(self, room: Any, event: Any) -> None:
        if getattr(room, "room_id", "") != self._room_id or self._handler is None:
            return

        await asyncio.to_thread(
            self._handler,
            {
                "room_id": getattr(room, "room_id", ""),
                "event_id": getattr(event, "event_id", ""),
                "sender_id": getattr(event, "sender", ""),
                "timestamp": str(
                    getattr(
                        event,
                        "server_timestamp",
                        getattr(event, "origin_server_ts", ""),
                    )
                ),
                "text": getattr(event, "body", ""),
                "message_type": "m.text",
            },
        )

    async def _send_text(self, *, room_id: str, event_id: str, text: str) -> Any:
        return await self._client.room_send(
            room_id=room_id,
            message_type="m.room.message",
            content={
                "msgtype": "m.text",
                "body": text,
                "m.relates_to": {
                    "m.in_reply_to": {
                        "event_id": event_id,
                    }
                },
            },
        )


def build_bot(
    config: Config,
    *,
    bot_user_id: str = "",
    reply_sink: Any = None,
    archive_path: str | Path = "/data/archive/events.jsonl",
    memory_client: Any | None = None,
) -> MatrixMemoryBot:
    if memory_client is None:
        memory_client = MemoryClient.from_config(config)
    return MatrixMemoryBot(
        room_id=config.matrix_room_id,
        bot_user_id=bot_user_id,
        archive_path=archive_path,
        memory_client=memory_client,
        reply_sink=reply_sink,
    )


def run_bot(
    *,
    config: Config,
    gateway: Any,
    memory_client: Any | None = None,
    reply_sink: Any = None,
    archive_path: str | Path = "/data/archive/events.jsonl",
) -> MatrixMemoryBot:
    effective_reply_sink = reply_sink
    if effective_reply_sink is None and hasattr(gateway, "reply"):
        effective_reply_sink = gateway.reply

    bot = build_bot(
        config,
        reply_sink=effective_reply_sink,
        archive_path=archive_path,
        memory_client=memory_client,
    )
    bot.subscribe(gateway)
    gateway.run_forever()
    return bot


def main() -> int:
    config = load_config()
    gateway = NioMatrixGateway(
        homeserver_url=config.matrix_homeserver_url,
        access_token=config.matrix_access_token,
    )
    run_bot(config=config, gateway=gateway)
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
