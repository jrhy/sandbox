import os
from pathlib import Path
import tempfile
import unittest
from unittest.mock import patch

from mem0fun.matrixmem import config as config_module
from mem0fun.matrixmem.config import ConfigError, load_config


class ConfigTest(unittest.TestCase):
    def test_load_config_reads_required_settings(self):
        config = load_config(
            {
                "MATRIX_HOMESERVER_URL": "https://matrix.example",
                "MATRIX_ACCESS_TOKEN": "secret-token",
                "MATRIX_ROOM_ID": "!room:example",
                "OLLAMA_BASE_URL": "http://host.docker.internal:11434",
                "QDRANT_URL": "http://qdrant:6333",
            }
        )

        self.assertEqual(config.matrix_homeserver_url, "https://matrix.example")
        self.assertEqual(config.matrix_access_token, "secret-token")
        self.assertEqual(config.matrix_room_id, "!room:example")
        self.assertEqual(config.ollama_base_url, "http://host.docker.internal:11434")
        self.assertEqual(config.qdrant_url, "http://qdrant:6333")
        self.assertEqual(config.kuzu_path, "/data/kuzu/matrixmem.kuzu")
        self.assertEqual(config.mem0_collection_name, "matrixmem")
        self.assertEqual(config.ollama_chat_model, "qwen3.5:9b")
        self.assertEqual(config.ollama_embed_model, "mxbai-embed-large")
        self.assertEqual(config.ollama_embedding_dims, 1024)

    def test_load_config_reports_missing_required_settings(self):
        with self.assertRaises(ConfigError) as exc_info:
            load_config({})

        message = str(exc_info.exception)
        self.assertIn("Missing required configuration", message)
        self.assertIn("MATRIX_HOMESERVER_URL", message)
        self.assertIn("MATRIX_ACCESS_TOKEN", message)
        self.assertIn("MATRIX_ROOM_ID", message)
        self.assertIn("OLLAMA_BASE_URL", message)
        self.assertIn("QDRANT_URL", message)

    def test_load_config_uses_optional_defaults_when_not_provided(self):
        config = load_config(
            {
                "MATRIX_HOMESERVER_URL": "https://matrix.example",
                "MATRIX_ACCESS_TOKEN": "secret-token",
                "MATRIX_ROOM_ID": "!room:example",
                "OLLAMA_BASE_URL": "http://host.docker.internal:11434",
                "QDRANT_URL": "http://qdrant:6333",
            }
        )

        self.assertEqual(config.kuzu_path, "/data/kuzu/matrixmem.kuzu")
        self.assertEqual(config.mem0_collection_name, "matrixmem")
        self.assertEqual(config.ollama_chat_model, "qwen3.5:9b")
        self.assertEqual(config.ollama_embed_model, "mxbai-embed-large")
        self.assertEqual(config.ollama_embedding_dims, 1024)

    def test_load_config_reads_local_dotenv_and_prefers_real_environment(self):
        with tempfile.TemporaryDirectory() as temp_dir:
            temp_path = Path(temp_dir)
            (temp_path / ".env").write_text(
                "\n".join(
                    [
                        "MATRIX_HOMESERVER_URL=https://matrix.example",
                        "MATRIX_ACCESS_TOKEN=dotenv-token",
                        "MATRIX_ROOM_ID=!room:example",
                        "OLLAMA_BASE_URL=http://host.docker.internal:11434",
                        "QDRANT_URL=http://qdrant:6333",
                    ]
                )
                + "\n",
                encoding="utf-8",
            )

            with patch.object(
                config_module,
                "__file__",
                str(temp_path / "config.py"),
            ):
                with patch.dict(
                    os.environ,
                    {
                        "MATRIX_ACCESS_TOKEN": "env-token",
                    },
                    clear=True,
                ):
                    config = load_config()

        self.assertEqual(config.matrix_homeserver_url, "https://matrix.example")
        self.assertEqual(config.matrix_access_token, "env-token")
        self.assertEqual(config.matrix_room_id, "!room:example")
        self.assertEqual(config.ollama_base_url, "http://host.docker.internal:11434")
        self.assertEqual(config.qdrant_url, "http://qdrant:6333")
        self.assertEqual(config.kuzu_path, "/data/kuzu/matrixmem.kuzu")
        self.assertEqual(config.mem0_collection_name, "matrixmem")
        self.assertEqual(config.ollama_chat_model, "qwen3.5:9b")
        self.assertEqual(config.ollama_embed_model, "mxbai-embed-large")
        self.assertEqual(config.ollama_embedding_dims, 1024)

if __name__ == "__main__":
    unittest.main()
