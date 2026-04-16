from __future__ import annotations

from dataclasses import dataclass
import re
from typing import Any, Mapping

from .config import (
    Config,
    DEFAULT_KUZU_PATH,
    DEFAULT_MEM0_COLLECTION_NAME,
    DEFAULT_OLLAMA_CHAT_MODEL,
    DEFAULT_OLLAMA_EMBEDDING_DIMS,
    DEFAULT_OLLAMA_EMBED_MODEL,
)
from . import prompting

try:
    from mem0 import Memory
except ImportError:  # pragma: no cover - exercised only when dependency is absent.
    class Memory:  # type: ignore[no-redef]
        @classmethod
        def from_config(cls, config: Mapping[str, Any]):
            raise RuntimeError("mem0 is not installed")


@dataclass(frozen=True, slots=True)
class MemoryClientConfig:
    qdrant_url: str
    kuzu_path: str
    ollama_base_url: str
    collection_name: str = DEFAULT_MEM0_COLLECTION_NAME
    chat_model: str = DEFAULT_OLLAMA_CHAT_MODEL
    embed_model: str = DEFAULT_OLLAMA_EMBED_MODEL
    embedding_dims: int = DEFAULT_OLLAMA_EMBEDDING_DIMS


class MemoryClient:
    def __init__(
        self,
        *,
        qdrant_url: str,
        kuzu_path: str = DEFAULT_KUZU_PATH,
        ollama_base_url: str,
        collection_name: str = DEFAULT_MEM0_COLLECTION_NAME,
        chat_model: str = DEFAULT_OLLAMA_CHAT_MODEL,
        embed_model: str = DEFAULT_OLLAMA_EMBED_MODEL,
        embedding_dims: int = DEFAULT_OLLAMA_EMBEDDING_DIMS,
    ) -> None:
        self.config = MemoryClientConfig(
            qdrant_url=qdrant_url,
            kuzu_path=kuzu_path,
            ollama_base_url=ollama_base_url,
            collection_name=collection_name,
            chat_model=chat_model,
            embed_model=embed_model,
            embedding_dims=embedding_dims,
        )
        self.memory = Memory.from_config(self._build_mem0_config())

    @classmethod
    def from_config(cls, config: Config) -> "MemoryClient":
        return cls(
            qdrant_url=config.qdrant_url,
            kuzu_path=config.kuzu_path,
            ollama_base_url=config.ollama_base_url,
            collection_name=config.mem0_collection_name,
            chat_model=config.ollama_chat_model,
            embed_model=config.ollama_embed_model,
            embedding_dims=config.ollama_embedding_dims,
        )

    def add_message(
        self,
        *,
        text: str,
        user_id: str,
        room_id: str,
        event_id: str,
        sender_id: str,
        timestamp: str,
        handling_mode: str,
        metadata: Mapping[str, Any] | None = None,
    ) -> Any:
        payload_metadata = self._build_message_metadata(
            room_id=room_id,
            event_id=event_id,
            sender_id=sender_id,
            timestamp=timestamp,
            handling_mode=handling_mode,
            metadata=metadata,
        )
        messages = [{"role": "user", "content": text}]
        return self.memory.add(messages, user_id=user_id, metadata=payload_metadata)

    def answer_query(
        self,
        query: str,
        *,
        user_id: str,
        room_id: str,
        metadata: Mapping[str, Any] | None = None,
    ) -> Any:
        search_metadata = self._normalize_metadata(metadata)
        search_metadata["source_room_id"] = room_id
        search_result = self.memory.search(query, user_id=user_id, filters=search_metadata)
        results = self._normalize_search_results(search_result.get("results"))
        relations = self._normalize_search_results(search_result.get("relations"))
        prompt = prompting.build_query_prompt(
            query,
            memories=results,
            relations=relations,
        )

        try:
            answer = prompting.synthesize_query_answer(prompt)
        except prompting.NoUsableContextError:
            answer = prompting.FALLBACK_QUERY_ANSWER

        return {
            "answer": answer,
            "results": results,
            "relations": relations,
        }

    def _build_mem0_config(self) -> dict[str, Any]:
        return {
            "vector_store": {
                "provider": "qdrant",
                "config": {
                    "url": self.config.qdrant_url,
                    "collection_name": self.config.collection_name,
                    "embedding_model_dims": self.config.embedding_dims,
                },
            },
            "graph_store": {
                "provider": "kuzu",
                "config": {
                    "db": self.config.kuzu_path,
                },
            },
            "embedder": {
                "provider": "ollama",
                "config": {
                    "ollama_base_url": self.config.ollama_base_url,
                    "model": self.config.embed_model,
                    "embedding_dims": self.config.embedding_dims,
                },
            },
            "llm": {
                "provider": "ollama",
                "config": {
                    "ollama_base_url": self.config.ollama_base_url,
                    "model": self.config.chat_model,
                    "temperature": 0,
                    "max_tokens": 2000,
                },
            },
        }

    def _build_message_metadata(
        self,
        *,
        room_id: str,
        event_id: str,
        sender_id: str,
        timestamp: str,
        handling_mode: str,
        metadata: Mapping[str, Any] | None,
    ) -> dict[str, Any]:
        normalized = self._normalize_metadata(metadata)
        normalized.update(
            {
                "handling_mode": handling_mode,
                "source_room_id": room_id,
                "source_event_id": event_id,
                "source_sender_id": sender_id,
                "source_timestamp": timestamp,
            }
        )
        return normalized

    def _normalize_metadata(self, metadata: Mapping[str, Any] | None) -> dict[str, Any]:
        if not metadata:
            return {}

        normalized: dict[str, Any] = {}
        for key, value in metadata.items():
            normalized[self._normalize_key(str(key))] = self._normalize_value(value)
        return normalized

    def _normalize_key(self, key: str) -> str:
        cleaned = re.sub(r"[^a-zA-Z0-9]+", "_", key.strip().lower())
        return cleaned.strip("_")

    def _normalize_value(self, value: Any) -> Any:
        if isinstance(value, str):
            return value.strip()
        return value

    def _normalize_search_results(self, items: Any) -> list[Any]:
        if items is None:
            return []
        if isinstance(items, list):
            return items
        return list(items)
