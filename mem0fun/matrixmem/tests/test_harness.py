from __future__ import annotations

from pathlib import Path
import tempfile
import unittest

from mem0fun.matrixmem.harness import run_harness


class HarnessTest(unittest.TestCase):
    def test_mock_mode_runs_statement_then_query_and_returns_summary(self):
        with tempfile.TemporaryDirectory() as tmpdir:
            archive_path = Path(tmpdir) / "events.jsonl"

            result = run_harness(
                mode="mock",
                archive_path=archive_path,
                statement_text="Need to buy solder wick",
                query_text="What should I buy?",
                room_id="!room:example",
                user_id="@alice:example",
            )

            self.assertEqual(result["mode"], "mock")
            self.assertEqual(result["reply"]["text"], "You said to buy: Need to buy solder wick.")
            self.assertEqual(
                result["archive_records"],
                [
                    {
                        "room_id": "!room:example",
                        "event_id": "$statement",
                        "timestamp": "2026-04-12T00:00:00+00:00",
                        "text": "Need to buy solder wick",
                        "sender_id": "@alice:example",
                        "handling_mode": "ingest",
                    },
                    {
                        "room_id": "!room:example",
                        "event_id": "$query",
                        "timestamp": "2026-04-12T00:00:01+00:00",
                        "text": "What should I buy?",
                        "sender_id": "@alice:example",
                        "handling_mode": "query",
                    },
                ],
            )
            self.assertEqual(
                result["memory_client"]["add_calls"],
                [
                    {
                        "text": "Need to buy solder wick",
                        "user_id": "@alice:example",
                        "room_id": "!room:example",
                        "event_id": "$statement",
                        "sender_id": "@alice:example",
                        "timestamp": "2026-04-12T00:00:00+00:00",
                        "handling_mode": "ingest",
                    }
                ],
            )
            self.assertEqual(
                result["memory_client"]["query_calls"],
                [
                    {
                        "query": "What should I buy?",
                        "user_id": "@alice:example",
                        "room_id": "!room:example",
                    }
                ],
            )
            self.assertEqual(
                result["memory_client"]["stored_memories"],
                [
                    {
                        "text": "Need to buy solder wick",
                        "user_id": "@alice:example",
                        "room_id": "!room:example",
                        "event_id": "$statement",
                        "sender_id": "@alice:example",
                        "timestamp": "2026-04-12T00:00:00+00:00",
                        "handling_mode": "ingest",
                    }
                ],
            )
            self.assertEqual(
                result["query_result"]["results"],
                [
                    {
                        "memory": "Need to buy solder wick",
                        "score": 1.0,
                    }
                ],
            )

    def test_mock_mode_without_explicit_archive_path_cleans_up_temp_dir(self):
        result = run_harness(mode="mock")
        self.assertFalse(Path(result["archive_path"]).exists())


if __name__ == "__main__":
    unittest.main()
