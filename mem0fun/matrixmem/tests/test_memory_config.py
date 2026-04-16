import unittest
from unittest.mock import patch

from mem0fun.matrixmem import config as config_module
import mem0fun.matrixmem.prompting as prompting_module
import mem0fun.matrixmem.memory as memory_module


class RecordingMemory:
    def __init__(self):
        self.add_calls = []
        self.search_calls = []

    @classmethod
    def from_config(cls, config):
        instance = cls()
        instance.config = config
        return instance

    def add(self, messages, user_id=None, metadata=None):
        self.add_calls.append(
            {
                "messages": messages,
                "user_id": user_id,
                "metadata": metadata,
            }
        )
        return {"messages": messages, "user_id": user_id, "metadata": metadata}

    def search(self, query, user_id=None, filters=None, limit=100, threshold=None, rerank=True):
        del limit, threshold, rerank
        self.search_calls.append(
            {
                "query": query,
                "user_id": user_id,
                "filters": filters,
            }
        )
        return {"results": []}


class MemoryConfigTest(unittest.TestCase):
    def test_client_builds_mem0_config_with_qdrant_kuzu_and_ollama(self):
        with patch.object(memory_module, "Memory", RecordingMemory):
            config = config_module.Config(
                matrix_homeserver_url="https://matrix.example",
                matrix_access_token="secret-token",
                matrix_room_id="!room:example",
                ollama_base_url="http://host.docker.internal:11434",
                qdrant_url="http://qdrant:6333",
                kuzu_path="/data/kuzu/matrixmem.kuzu",
                mem0_collection_name="matrixmem",
                ollama_chat_model="qwen3.5:9b",
                ollama_embed_model="mxbai-embed-large",
                ollama_embedding_dims=1024,
            )
            client = memory_module.MemoryClient.from_config(config)

        config = client.memory.config
        self.assertEqual(config["vector_store"]["provider"], "qdrant")
        self.assertEqual(config["vector_store"]["config"]["url"], "http://qdrant:6333")
        self.assertEqual(config["vector_store"]["config"]["collection_name"], "matrixmem")
        self.assertEqual(config["vector_store"]["config"]["embedding_model_dims"], 1024)

        self.assertEqual(config["graph_store"]["provider"], "kuzu")
        self.assertEqual(config["graph_store"]["config"]["db"], "/data/kuzu/matrixmem.kuzu")

        self.assertEqual(config["llm"]["provider"], "ollama")
        self.assertEqual(
            config["llm"]["config"]["ollama_base_url"],
            "http://host.docker.internal:11434",
        )
        self.assertEqual(config["llm"]["config"]["model"], "qwen3.5:9b")

        self.assertEqual(config["embedder"]["provider"], "ollama")
        self.assertEqual(
            config["embedder"]["config"]["ollama_base_url"],
            "http://host.docker.internal:11434",
        )
        self.assertEqual(config["embedder"]["config"]["model"], "mxbai-embed-large")
        self.assertEqual(config["embedder"]["config"]["embedding_dims"], 1024)

    def test_client_uses_stable_default_kuzu_path_when_not_provided(self):
        with patch.object(memory_module, "Memory", RecordingMemory):
            config = config_module.Config(
                matrix_homeserver_url="https://matrix.example",
                matrix_access_token="secret-token",
                matrix_room_id="!room:example",
                ollama_base_url="http://host.docker.internal:11434",
                qdrant_url="http://qdrant:6333",
            )
            client = memory_module.MemoryClient.from_config(config)

        self.assertEqual(client.config.kuzu_path, "/data/kuzu/matrixmem.kuzu")
        self.assertEqual(
            client.memory.config["graph_store"]["config"]["db"],
            "/data/kuzu/matrixmem.kuzu",
        )

    def test_add_message_normalizes_metadata_and_adds_source_attribution(self):
        with patch.object(memory_module, "Memory", RecordingMemory):
            config = config_module.Config(
                matrix_homeserver_url="https://matrix.example",
                matrix_access_token="secret-token",
                matrix_room_id="!room:example",
                ollama_base_url="http://host.docker.internal:11434",
                qdrant_url="http://qdrant:6333",
                kuzu_path="/data/kuzu/matrixmem.kuzu",
            )
            client = memory_module.MemoryClient.from_config(config)

            result = client.add_message(
                text="Need solder wick",
                user_id="alice",
                room_id="!room:example",
                event_id="$event-1",
                sender_id="@alice:example",
                timestamp="2026-04-11T23:07:32.874775+00:00",
                handling_mode="ingest",
                metadata={"Category": "Electronics", "Project Name": "Bench repair"},
            )

        self.assertEqual(result["metadata"]["category"], "Electronics")
        self.assertEqual(result["metadata"]["project_name"], "Bench repair")
        self.assertEqual(result["metadata"]["handling_mode"], "ingest")
        self.assertEqual(result["metadata"]["source_room_id"], "!room:example")
        self.assertEqual(result["metadata"]["source_event_id"], "$event-1")
        self.assertEqual(result["metadata"]["source_sender_id"], "@alice:example")
        self.assertEqual(
            result["metadata"]["source_timestamp"],
            "2026-04-11T23:07:32.874775+00:00",
        )
        self.assertEqual(result["messages"], [{"role": "user", "content": "Need solder wick"}])
        self.assertEqual(result["user_id"], "alice")

    def test_answer_query_is_a_minimal_search_wrapper(self):
        with patch.object(memory_module, "Memory", RecordingMemory):
            config = config_module.Config(
                matrix_homeserver_url="https://matrix.example",
                matrix_access_token="secret-token",
                matrix_room_id="!room:example",
                ollama_base_url="http://host.docker.internal:11434",
                qdrant_url="http://qdrant:6333",
                kuzu_path="/data/kuzu/matrixmem.kuzu",
            )
            client = memory_module.MemoryClient.from_config(config)

            result = client.answer_query(
                "What should I do next?",
                user_id="alice",
                room_id="!room:example",
            )

        self.assertEqual(result["answer"], prompting_module.FALLBACK_QUERY_ANSWER)
        self.assertEqual(result["results"], [])
        self.assertEqual(result["relations"], [])
        self.assertEqual(
            client.memory.search_calls,
            [
                {
                    "query": "What should I do next?",
                    "user_id": "alice",
                    "filters": {"source_room_id": "!room:example"},
                }
            ],
        )

    def test_answer_query_scopes_search_to_room_and_merges_metadata(self):
        with patch.object(memory_module, "Memory", RecordingMemory):
            config = config_module.Config(
                matrix_homeserver_url="https://matrix.example",
                matrix_access_token="secret-token",
                matrix_room_id="!room:example",
                ollama_base_url="http://host.docker.internal:11434",
                qdrant_url="http://qdrant:6333",
            )
            client = memory_module.MemoryClient.from_config(config)

            result = client.answer_query(
                "What should I do next?",
                user_id="alice",
                room_id="!room:example",
                metadata={"kind": "question", "priority": "high"},
            )

        self.assertEqual(result["answer"], prompting_module.FALLBACK_QUERY_ANSWER)
        self.assertEqual(result["results"], [])
        self.assertEqual(result["relations"], [])
        self.assertEqual(
            client.memory.search_calls,
            [
                {
                    "query": "What should I do next?",
                    "user_id": "alice",
                    "filters": {
                        "source_room_id": "!room:example",
                        "kind": "question",
                        "priority": "high",
                    },
                }
            ],
        )


if __name__ == "__main__":
    unittest.main()
