import os
from pprint import pprint

from mem0 import Memory


def make_memory() -> Memory:
    ollama_host = os.environ.get("OLLAMA_HOST", "http://host.docker.internal:11434")
    qdrant_url = os.environ.get("QDRANT_URL", "http://qdrant:6333")
    embed_model = os.environ.get("OLLAMA_EMBED_MODEL", "mxbai-embed-large")
    chat_model = os.environ.get("OLLAMA_CHAT_MODEL", "qwen3.5:9b")
    collection_name = os.environ.get("MEM0_COLLECTION_NAME", "mem0_quickstart")

    config = {
        "vector_store": {
            "provider": "qdrant",
            "config": {
                "url": qdrant_url,
                "collection_name": collection_name,
                "embedding_model_dims": 1024,
            },
        },
        "embedder": {
            "provider": "ollama",
            "config": {
                "ollama_base_url": ollama_host,
                "model": embed_model,
                "embedding_dims": 1024,
            },
        },
        "llm": {
            "provider": "ollama",
            "config": {
                "ollama_base_url": ollama_host,
                "model": chat_model,
                "temperature": 0,
                "max_tokens": 2000,
            },
        },
    }

    return Memory.from_config(config)


def main() -> None:
    memory = make_memory()
    user_id = os.environ.get("MEM0_USER_ID", "alice")

    messages = [
        {"role": "user", "content": "I'm planning to watch a movie tonight. Any recommendations?"},
        {"role": "assistant", "content": "How about a thriller? They can be quite engaging."},
        {"role": "user", "content": "I'm not a big fan of thrillers but I love sci-fi movies."},
        {"role": "assistant", "content": "Got it. I'll avoid thriller recommendations and suggest sci-fi movies in the future."},
    ]

    print("Adding memory...")
    add_result = memory.add(messages, user_id=user_id, metadata={"category": "movie_recommendations"})
    pprint(add_result)

    print("\nSearching memory...")
    search_result = memory.search(
        "What kind of movies does the user like?",
        user_id=user_id,
    )
    pprint(search_result)


if __name__ == "__main__":
    main()
