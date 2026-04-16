from __future__ import annotations

import asyncio
from pathlib import Path
import json
import tempfile
import threading
import unittest

from mem0fun.matrixmem.bot import BotMessage, MatrixMemoryBot
from mem0fun.matrixmem.config import Config
from mem0fun.matrixmem.main import NioMatrixGateway, run_bot


class RecordingMemoryClient:
    def __init__(self):
        self.add_calls = []
        self.query_calls = []

    def add_message(
        self,
        *,
        text,
        user_id,
        room_id,
        event_id,
        sender_id,
        timestamp,
        handling_mode,
        metadata=None,
    ):
        call = {
            "text": text,
            "user_id": user_id,
            "room_id": room_id,
            "event_id": event_id,
            "sender_id": sender_id,
            "timestamp": timestamp,
            "handling_mode": handling_mode,
            "metadata": metadata,
        }
        self.add_calls.append(call)
        return call

    def answer_query(self, query, *, user_id, room_id, metadata=None):
        call = {
            "query": query,
            "user_id": user_id,
            "room_id": room_id,
            "metadata": metadata,
        }
        self.query_calls.append(call)
        return {
            "answer": "Use low-energy chores before starting a new project.",
            "results": [{"memory": "Do taxes"}],
            "relations": [{"source": "taxes", "relation": "blocked_by", "target": "receipts"}],
        }


class RecordingReplySink:
    def __init__(self):
        self.calls = []

    def __call__(self, *, room_id, event_id, text):
        self.calls.append(
            {
                "room_id": room_id,
                "event_id": event_id,
                "text": text,
            }
        )


class RecordingGateway:
    def __init__(self):
        self.subscriptions = []
        self.run_calls = 0

    def subscribe_room(self, room_id, handler):
        self.subscriptions.append({"room_id": room_id, "handler": handler})
        return {"room_id": room_id}

    def run_forever(self):
        self.run_calls += 1


class FakeAsyncClient:
    def __init__(self, *, room_send_error=None):
        self.access_token = None
        self.user = ""
        self.callback_calls = []
        self.sync_forever_calls = []
        self.room_send_calls = []
        self.whoami_calls = 0
        self.room_send_error = room_send_error

    def add_event_callback(self, callback, event_type):
        self.callback_calls.append({"callback": callback, "event_type": event_type})

    async def whoami(self):
        self.whoami_calls += 1
        return type("WhoamiResponse", (), {"user_id": "@bot:example"})()

    async def sync_forever(self, timeout=None, sync_filter=None):
        self.sync_forever_calls.append(
            {
                "timeout": timeout,
                "sync_filter": sync_filter,
            }
        )

    async def room_send(
        self,
        room_id,
        message_type,
        content,
        tx_id=None,
        ignore_unverified_devices=False,
    ):
        self.room_send_calls.append(
            {
                "room_id": room_id,
                "message_type": message_type,
                "content": content,
                "tx_id": tx_id,
                "ignore_unverified_devices": ignore_unverified_devices,
            }
        )
        if self.room_send_error is not None:
            raise self.room_send_error
        return {"event_id": "$reply"}


class BotFlowTest(unittest.TestCase):
    def test_ingest_message_archives_and_stores(self):
        with tempfile.TemporaryDirectory() as tmpdir:
            archive_path = Path(tmpdir) / "events.jsonl"
            memory_client = RecordingMemoryClient()
            reply_sink = RecordingReplySink()
            bot = MatrixMemoryBot(
                room_id="!room:example",
                bot_user_id="@bot:example",
                archive_path=archive_path,
                memory_client=memory_client,
                reply_sink=reply_sink,
            )

            result = bot.handle_message(
                BotMessage(
                    room_id="!room:example",
                    event_id="$event-1",
                    sender_id="@alice:example",
                    timestamp="2026-04-11T23:07:32.874775+00:00",
                    text="Planning to watch a movie tonight",
                )
            )

            self.assertEqual(result["handling_mode"], "ingest")
            self.assertEqual(len(memory_client.add_calls), 1)
            self.assertEqual(memory_client.query_calls, [])
            self.assertEqual(reply_sink.calls, [])
            self.assertTrue(archive_path.exists())

            archive_lines = archive_path.read_text(encoding="utf-8").splitlines()
            self.assertEqual(len(archive_lines), 1)
            self.assertEqual(
                json.loads(archive_lines[0]),
                {
                    "room_id": "!room:example",
                    "event_id": "$event-1",
                    "timestamp": "2026-04-11T23:07:32.874775+00:00",
                    "text": "Planning to watch a movie tonight",
                    "sender_id": "@alice:example",
                    "handling_mode": "ingest",
                },
            )
            self.assertEqual(
                memory_client.add_calls[0],
                {
                    "text": "Planning to watch a movie tonight",
                    "user_id": "@alice:example",
                    "room_id": "!room:example",
                    "event_id": "$event-1",
                    "sender_id": "@alice:example",
                    "timestamp": "2026-04-11T23:07:32.874775+00:00",
                    "handling_mode": "ingest",
                    "metadata": None,
                },
            )

    def test_query_message_archives_and_replies(self):
        with tempfile.TemporaryDirectory() as tmpdir:
            archive_path = Path(tmpdir) / "events.jsonl"
            memory_client = RecordingMemoryClient()
            reply_sink = RecordingReplySink()
            bot = MatrixMemoryBot(
                room_id="!room:example",
                bot_user_id="@bot:example",
                archive_path=archive_path,
                memory_client=memory_client,
                reply_sink=reply_sink,
            )

            result = bot.handle_message(
                BotMessage(
                    room_id="!room:example",
                    event_id="$event-2",
                    sender_id="@alice:example",
                    timestamp="2026-04-11T23:07:33.000000+00:00",
                    text="What should I do next?",
                )
            )

            self.assertEqual(result["handling_mode"], "query")
            self.assertEqual(len(memory_client.query_calls), 1)
            self.assertEqual(memory_client.add_calls, [])
            self.assertEqual(reply_sink.calls, [{"room_id": "!room:example", "event_id": "$event-2", "text": "Use low-energy chores before starting a new project."}])

            archive_lines = archive_path.read_text(encoding="utf-8").splitlines()
            self.assertEqual(len(archive_lines), 1)
            self.assertEqual(
                json.loads(archive_lines[0]),
                {
                    "room_id": "!room:example",
                    "event_id": "$event-2",
                    "timestamp": "2026-04-11T23:07:33.000000+00:00",
                    "text": "What should I do next?",
                    "sender_id": "@alice:example",
                    "handling_mode": "query",
                },
            )
            self.assertEqual(
                memory_client.query_calls[0],
                {
                    "query": "What should I do next?",
                    "user_id": "@alice:example",
                    "room_id": "!room:example",
                    "metadata": None,
                },
            )

    def test_bot_message_is_ignored(self):
        with tempfile.TemporaryDirectory() as tmpdir:
            archive_path = Path(tmpdir) / "events.jsonl"
            memory_client = RecordingMemoryClient()
            reply_sink = RecordingReplySink()
            bot = MatrixMemoryBot(
                room_id="!room:example",
                bot_user_id="@bot:example",
                archive_path=archive_path,
                memory_client=memory_client,
                reply_sink=reply_sink,
            )

            result = bot.handle_message(
                BotMessage(
                    room_id="!room:example",
                    event_id="$event-3",
                    sender_id="@bot:example",
                    timestamp="2026-04-11T23:07:34.000000+00:00",
                    text="I should not store my own replies",
                )
            )

            self.assertEqual(result["handling_mode"], "ignore")
            self.assertFalse(archive_path.exists())
            self.assertEqual(memory_client.add_calls, [])
            self.assertEqual(memory_client.query_calls, [])
            self.assertEqual(reply_sink.calls, [])

    def test_non_text_message_is_ignored(self):
        with tempfile.TemporaryDirectory() as tmpdir:
            archive_path = Path(tmpdir) / "events.jsonl"
            memory_client = RecordingMemoryClient()
            reply_sink = RecordingReplySink()
            bot = MatrixMemoryBot(
                room_id="!room:example",
                bot_user_id="@bot:example",
                archive_path=archive_path,
                memory_client=memory_client,
                reply_sink=reply_sink,
            )

            result = bot.handle_event(
                {
                    "room_id": "!room:example",
                    "event_id": "$event-4",
                    "sender_id": "@alice:example",
                    "timestamp": "2026-04-11T23:07:35.000000+00:00",
                    "type": "m.room.message",
                    "content": {
                        "body": "Picture of the workbench",
                        "msgtype": "m.image",
                    },
                }
            )

            self.assertEqual(result["handling_mode"], "ignore")
            self.assertFalse(archive_path.exists())
            self.assertEqual(memory_client.add_calls, [])
            self.assertEqual(memory_client.query_calls, [])
            self.assertEqual(reply_sink.calls, [])


class MainFlowTest(unittest.TestCase):
    def test_run_bot_subscribes_room_and_starts_gateway_loop(self):
        with tempfile.TemporaryDirectory() as tmpdir:
            archive_path = Path(tmpdir) / "events.jsonl"
            memory_client = RecordingMemoryClient()
            reply_sink = RecordingReplySink()
            gateway = RecordingGateway()

            bot = run_bot(
                config=Config(
                    matrix_homeserver_url="https://matrix.example",
                    matrix_access_token="secret-token",
                    matrix_room_id="!room:example",
                    ollama_base_url="http://ollama:11434",
                    qdrant_url="http://qdrant:6333",
                ),
                gateway=gateway,
                memory_client=memory_client,
                reply_sink=reply_sink,
                archive_path=archive_path,
            )

            self.assertEqual(len(gateway.subscriptions), 1)
            self.assertEqual(gateway.subscriptions[0]["room_id"], "!room:example")
            self.assertIs(gateway.subscriptions[0]["handler"].__self__, bot)
            self.assertEqual(gateway.subscriptions[0]["handler"].__func__, bot.handle_event.__func__)
            self.assertEqual(gateway.run_calls, 1)

    def test_gateway_sends_matrix_reply_relation(self):
        fake_client = FakeAsyncClient()
        gateway = NioMatrixGateway(
            homeserver_url="https://matrix.example",
            access_token="secret-token",
            client=fake_client,
        )

        asyncio.run(
            gateway._send_text(
                room_id="!room:example",
                event_id="$event-2",
                text="Use low-energy chores before starting a new project.",
            )
        )

        self.assertEqual(
            fake_client.room_send_calls,
            [
                {
                    "room_id": "!room:example",
                    "message_type": "m.room.message",
                    "content": {
                        "msgtype": "m.text",
                        "body": "Use low-energy chores before starting a new project.",
                        "m.relates_to": {
                            "m.in_reply_to": {
                                "event_id": "$event-2",
                            }
                        },
                    },
                    "tx_id": None,
                    "ignore_unverified_devices": False,
                }
            ],
        )

    def test_gateway_sync_forever_scopes_to_single_room(self):
        fake_client = FakeAsyncClient()
        message_type = object()
        gateway = NioMatrixGateway(
            homeserver_url="https://matrix.example",
            access_token="secret-token",
            client=fake_client,
            room_message_text_type=message_type,
        )

        gateway.subscribe_room("!room:example", lambda event: event)
        asyncio.run(gateway._run_forever())

        self.assertEqual(fake_client.whoami_calls, 1)
        self.assertEqual(fake_client.user, "@bot:example")
        self.assertEqual(len(fake_client.callback_calls), 1)
        self.assertIs(fake_client.callback_calls[0]["event_type"], message_type)
        self.assertEqual(
            fake_client.sync_forever_calls,
            [
                {
                    "timeout": 30_000,
                    "sync_filter": {
                        "room": {
                            "rooms": ["!room:example"],
                            "timeline": {
                                "types": ["m.room.message"],
                            },
                        }
                    },
                }
            ],
        )

    def test_gateway_reply_surfaces_send_failures(self):
        fake_client = FakeAsyncClient(room_send_error=RuntimeError("send failed"))
        gateway = NioMatrixGateway(
            homeserver_url="https://matrix.example",
            access_token="secret-token",
            client=fake_client,
        )

        async def exercise():
            gateway._loop = asyncio.get_running_loop()
            with self.assertRaisesRegex(RuntimeError, "send failed"):
                await asyncio.to_thread(
                    gateway.reply,
                    room_id="!room:example",
                    event_id="$event-2",
                    text="Use low-energy chores before starting a new project.",
                )

        asyncio.run(exercise())

    def test_gateway_offloads_handler_work_from_event_loop(self):
        fake_client = FakeAsyncClient()
        gateway = NioMatrixGateway(
            homeserver_url="https://matrix.example",
            access_token="secret-token",
            client=fake_client,
        )
        call_info = {}

        def handler(event):
            call_info["thread_id"] = threading.get_ident()
            call_info["event"] = event
            return {"handling_mode": "ingest"}

        gateway.subscribe_room("!room:example", handler)

        class Room:
            room_id = "!room:example"

        class Event:
            event_id = "$event-9"
            sender = "@alice:example"
            server_timestamp = 12345
            body = "Planning to watch a movie tonight"

        async def exercise():
            loop_thread_id = threading.get_ident()
            await gateway._on_room_message(Room(), Event())
            self.assertEqual(
                call_info["event"],
                {
                    "room_id": "!room:example",
                    "event_id": "$event-9",
                    "sender_id": "@alice:example",
                    "timestamp": "12345",
                    "text": "Planning to watch a movie tonight",
                    "message_type": "m.text",
                },
            )
            self.assertNotEqual(call_info["thread_id"], loop_thread_id)

        asyncio.run(exercise())


if __name__ == "__main__":
    unittest.main()
