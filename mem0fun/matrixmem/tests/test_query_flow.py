import unittest
from unittest.mock import patch

from mem0fun.matrixmem import config as config_module
import mem0fun.matrixmem.memory as memory_module
import mem0fun.matrixmem.prompting as prompting_module


class RecordingMemory:
    def __init__(self):
        self.search_calls = []

    @classmethod
    def from_config(cls, config):
        instance = cls()
        instance.config = config
        return instance

    def search(self, query, user_id=None, filters=None, limit=100, threshold=None, rerank=True):
        del limit, threshold, rerank
        self.search_calls.append(
            {
                "query": query,
                "user_id": user_id,
                "filters": filters,
            }
        )
        return {
            "results": [
                {
                    "memory": "Planning to watch a movie tonight",
                    "score": 0.91,
                },
                {
                    "memory": "Bring snacks for the movie",
                    "score": 0.84,
                },
                {
                    "memory": "Check if anyone wants to join",
                    "score": 0.79,
                },
                {
                    "memory": "Review the projector setup",
                    "score": 0.73,
                },
            ],
            "relations": [
                {
                    "source": "movie night",
                    "relation": "related_to",
                    "target": "sci-fi movies",
                },
                {
                    "source": "movie night",
                    "relation": "needs",
                    "target": "snacks",
                },
                {
                    "source": "movie night",
                    "relation": "depends_on",
                    "target": "free evening",
                },
            ],
        }


class QueryFlowTest(unittest.TestCase):
    def _build_client(self):
        with patch.object(memory_module, "Memory", RecordingMemory):
            config = config_module.Config(
                matrix_homeserver_url="https://matrix.example",
                matrix_access_token="secret-token",
                matrix_room_id="!room:example",
                ollama_base_url="http://host.docker.internal:11434",
                qdrant_url="http://qdrant:6333",
                kuzu_path="/data/kuzu/matrixmem.kuzu",
            )
            return memory_module.MemoryClient.from_config(config)

    def test_answer_query_uses_multiple_memories_and_question_frame(self):
        client = self._build_client()

        with patch.object(
            prompting_module,
            "synthesize_query_answer",
            wraps=prompting_module.synthesize_query_answer,
        ) as synthesize_mock:
            result = client.answer_query(
                "What should I do tonight?",
                user_id="alice",
                room_id="!room:example",
                metadata={"kind": "question"},
            )

        self.assertEqual(
            client.memory.search_calls,
            [
                {
                    "query": "What should I do tonight?",
                    "user_id": "alice",
                    "filters": {
                        "kind": "question",
                        "source_room_id": "!room:example",
                    },
                }
            ],
        )
        self.assertEqual(
            result["answer"],
            "For tonight, you should consider Planning to watch a movie tonight, "
            "Bring snacks for the movie, and Check if anyone wants to join, "
            "plus 1 more. "
            "Related graph context: movie night -> related_to -> sci-fi movies "
            "and movie night -> needs -> snacks, plus 1 more.",
        )
        self.assertEqual(
            result["results"],
            [
                {
                    "memory": "Planning to watch a movie tonight",
                    "score": 0.91,
                },
                {
                    "memory": "Bring snacks for the movie",
                    "score": 0.84,
                },
                {
                    "memory": "Check if anyone wants to join",
                    "score": 0.79,
                },
                {
                    "memory": "Review the projector setup",
                    "score": 0.73,
                },
            ],
        )
        self.assertEqual(
            result["relations"],
            [
                {
                    "source": "movie night",
                    "relation": "related_to",
                    "target": "sci-fi movies",
                },
                {
                    "source": "movie night",
                    "relation": "needs",
                    "target": "snacks",
                },
                {
                    "source": "movie night",
                    "relation": "depends_on",
                    "target": "free evening",
                },
            ],
        )
        captured_prompt = synthesize_mock.call_args.args[0]
        self.assertIn("Retrieved memories:", captured_prompt)
        self.assertIn("Graph relations:", captured_prompt)
        self.assertIn("Planning to watch a movie tonight", captured_prompt)
        self.assertIn("Bring snacks for the movie", captured_prompt)
        self.assertIn("movie night -> related_to -> sci-fi movies", captured_prompt)

        second_result = client.answer_query(
            "What electronics projects have I mentioned?",
            user_id="alice",
            room_id="!room:example",
        )
        self.assertTrue(
            second_result["answer"].startswith("A good place to start is "),
        )
        self.assertIn("plus 1 more", second_result["answer"])
        self.assertNotEqual(result["answer"], second_result["answer"])

    def test_answer_query_returns_fallback_when_synthesis_fails(self):
        client = self._build_client()

        with patch.object(
            prompting_module,
            "synthesize_query_answer",
            side_effect=prompting_module.NoUsableContextError("no context"),
        ):
            result = client.answer_query(
                "What should I do tonight?",
                user_id="alice",
                room_id="!room:example",
            )

        self.assertEqual(
            result["answer"],
            prompting_module.FALLBACK_QUERY_ANSWER,
        )
        self.assertEqual(
            result["results"],
            [
                {
                    "memory": "Planning to watch a movie tonight",
                    "score": 0.91,
                },
                {
                    "memory": "Bring snacks for the movie",
                    "score": 0.84,
                },
                {
                    "memory": "Check if anyone wants to join",
                    "score": 0.79,
                },
                {
                    "memory": "Review the projector setup",
                    "score": 0.73,
                },
            ],
        )
        self.assertEqual(
            result["relations"],
            [
                {
                    "source": "movie night",
                    "relation": "related_to",
                    "target": "sci-fi movies",
                },
                {
                    "source": "movie night",
                    "relation": "needs",
                    "target": "snacks",
                },
                {
                    "source": "movie night",
                    "relation": "depends_on",
                    "target": "free evening",
                },
            ],
        )

    def test_answer_query_allows_unexpected_synthesis_errors_to_surface(self):
        client = self._build_client()

        with patch.object(
            prompting_module,
            "synthesize_query_answer",
            side_effect=RuntimeError("boom"),
        ):
            with self.assertRaisesRegex(RuntimeError, "boom"):
                client.answer_query(
                    "What should I do tonight?",
                    user_id="alice",
                    room_id="!room:example",
                )


if __name__ == "__main__":
    unittest.main()
