from __future__ import annotations

import importlib.util
import os
from pathlib import Path
import tempfile
import time
import uuid
import unittest

from mem0fun.matrixmem.config import Config
from mem0fun.matrixmem.memory import MemoryClient
from mem0fun.matrixmem import prompting


MEM0_AVAILABLE = importlib.util.find_spec("mem0") is not None
REAL_MEM0_ENABLED = os.getenv("MEM0_REAL_INTEGRATION") == "1"


@unittest.skipUnless(
    MEM0_AVAILABLE and REAL_MEM0_ENABLED,
    "set MEM0_REAL_INTEGRATION=1 and install mem0 to run the real Mem0 integration test",
)
class RealMem0IntegrationTest(unittest.TestCase):
    def test_room_scoped_statement_is_searchable_with_real_mem0(self):
        qdrant_url = os.environ["QDRANT_URL"]
        ollama_base_url = os.environ["OLLAMA_BASE_URL"]
        room_id = "!integration:local"
        collection_name = f"matrixmem_integration_{uuid.uuid4().hex}"

        with tempfile.TemporaryDirectory() as tmpdir:
            client = MemoryClient(
                qdrant_url=qdrant_url,
                kuzu_path=str(Path(tmpdir) / "matrixmem.kuzu"),
                ollama_base_url=ollama_base_url,
                collection_name=collection_name,
            )

            client.add_message(
                text="Need to buy solder wick",
                user_id=room_id,
                room_id=room_id,
                event_id="$statement",
                sender_id="@alice:example",
                timestamp="2026-04-16T00:00:00+00:00",
                handling_mode="ingest",
            )

            deadline = time.monotonic() + 30
            result = None
            while time.monotonic() < deadline:
                result = client.answer_query(
                    "What should I buy?",
                    user_id=room_id,
                    room_id=room_id,
                )
                if result["results"]:
                    break
                time.sleep(1)

        self.assertIsNotNone(result)
        self.assertTrue(result["results"])
        self.assertEqual(result["results"][0]["memory"], "Need to buy solder wick")
        self.assertNotEqual(result["answer"], prompting.FALLBACK_QUERY_ANSWER)


if __name__ == "__main__":
    unittest.main()
