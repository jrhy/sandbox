from pathlib import Path
import json
import tempfile
import unittest

from mem0fun.matrixmem.archive import append_event


class ArchiveTest(unittest.TestCase):
    def test_append_event_writes_ndjson_records(self):
        with tempfile.TemporaryDirectory() as tmpdir:
            archive_path = Path(tmpdir) / "events.jsonl"

            append_event(
                archive_path,
                room_id="!room:example",
                event_id="$event-1",
                timestamp="2026-04-11T23:07:32.874775+00:00",
                text="Planning to watch a movie tonight",
                sender_id="@alice:example",
                handling_mode="ingest",
            )
            append_event(
                archive_path,
                room_id="!room:example",
                event_id="$event-2",
                timestamp="2026-04-11T23:07:33.000000+00:00",
                text="What should I do next?",
                sender_id="@alice:example",
                handling_mode="query",
            )

            self.assertTrue(archive_path.exists())
            lines = archive_path.read_text(encoding="utf-8").splitlines()
            self.assertEqual(len(lines), 2)

            first_record = json.loads(lines[0])
            second_record = json.loads(lines[1])

            self.assertEqual(
                first_record,
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
                second_record,
                {
                    "room_id": "!room:example",
                    "event_id": "$event-2",
                    "timestamp": "2026-04-11T23:07:33.000000+00:00",
                    "text": "What should I do next?",
                    "sender_id": "@alice:example",
                    "handling_mode": "query",
                },
            )


if __name__ == "__main__":
    unittest.main()
