from __future__ import annotations

from dataclasses import dataclass
import os
from pathlib import Path
from typing import Mapping

try:
    from dotenv import load_dotenv
except ImportError:  # pragma: no cover - exercised only when dependency is absent.
    load_dotenv = None


REQUIRED_ENV_VARS = (
    "MATRIX_HOMESERVER_URL",
    "MATRIX_ACCESS_TOKEN",
    "MATRIX_ROOM_ID",
    "OLLAMA_BASE_URL",
    "QDRANT_URL",
)

DEFAULT_KUZU_PATH = "/data/kuzu/matrixmem.kuzu"
DEFAULT_MEM0_COLLECTION_NAME = "matrixmem"
DEFAULT_OLLAMA_CHAT_MODEL = "qwen3.5:9b"
DEFAULT_OLLAMA_EMBED_MODEL = "mxbai-embed-large"
DEFAULT_OLLAMA_EMBEDDING_DIMS = 1024


class ConfigError(ValueError):
    pass


@dataclass(frozen=True, slots=True)
class Config:
    matrix_homeserver_url: str
    matrix_access_token: str
    matrix_room_id: str
    ollama_base_url: str
    qdrant_url: str
    kuzu_path: str = DEFAULT_KUZU_PATH
    mem0_collection_name: str = DEFAULT_MEM0_COLLECTION_NAME
    ollama_chat_model: str = DEFAULT_OLLAMA_CHAT_MODEL
    ollama_embed_model: str = DEFAULT_OLLAMA_EMBED_MODEL
    ollama_embedding_dims: int = DEFAULT_OLLAMA_EMBEDDING_DIMS


def load_config(env: Mapping[str, str] | None = None) -> Config:
    if env is None:
        _load_local_env_file()
        values = os.environ
    else:
        values = env

    missing = [name for name in REQUIRED_ENV_VARS if not values.get(name)]
    if missing:
        missing_vars = ", ".join(missing)
        raise ConfigError(f"Missing required configuration: {missing_vars}")

    return Config(
        matrix_homeserver_url=values["MATRIX_HOMESERVER_URL"],
        matrix_access_token=values["MATRIX_ACCESS_TOKEN"],
        matrix_room_id=values["MATRIX_ROOM_ID"],
        ollama_base_url=values["OLLAMA_BASE_URL"],
        qdrant_url=values["QDRANT_URL"],
        kuzu_path=values.get("KUZU_PATH", DEFAULT_KUZU_PATH),
        mem0_collection_name=values.get(
            "MEM0_COLLECTION_NAME",
            DEFAULT_MEM0_COLLECTION_NAME,
        ),
        ollama_chat_model=values.get(
            "OLLAMA_CHAT_MODEL",
            DEFAULT_OLLAMA_CHAT_MODEL,
        ),
        ollama_embed_model=values.get(
            "OLLAMA_EMBED_MODEL",
            DEFAULT_OLLAMA_EMBED_MODEL,
        ),
        ollama_embedding_dims=int(
            values.get("OLLAMA_EMBEDDING_DIMS", DEFAULT_OLLAMA_EMBEDDING_DIMS)
        ),
    )


def _load_local_env_file() -> None:
    dotenv_path = Path(__file__).with_name(".env")
    if load_dotenv is not None:
        load_dotenv(dotenv_path=dotenv_path, override=False)
        return

    if not dotenv_path.exists():
        return

    for line in dotenv_path.read_text(encoding="utf-8").splitlines():
        stripped = line.strip()
        if not stripped or stripped.startswith("#") or "=" not in stripped:
            continue

        key, value = stripped.split("=", 1)
        key = key.strip()
        value = value.strip().strip('"').strip("'")
        if key:
            os.environ.setdefault(key, value)
