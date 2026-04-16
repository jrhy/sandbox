# Mem0 Ollama + Qdrant Demo

This directory contains a persistent Debian-based Docker Compose setup for trying the Mem0 Python quickstart with local Ollama and Qdrant.

## What it uses

- Ollama on your host at `http://host.docker.internal:11434`
- `mxbai-embed-large` for embeddings
- `qwen3.5:9b` for the LLM side
- Qdrant in Docker for vector storage

## Start the stack

```bash
cd /Users/jeffr/sandbox/mem0fun
docker compose up -d qdrant
./run-demo.sh
```

If you want to watch the app container output directly:

```bash
docker compose run --name mem0fun-app app python3 /workspace/demo.py
```

## Notes

- The app container uses an internal Docker network so it does not need outbound internet access.
- The only external dependency is your local Ollama process on port `11434`.
- Qdrant data persists in the named volume `qdrant_data`.
- Demo output is written to `logs/demo-*.log` when you use `run-demo.sh`.

## If Ollama needs a different model

You can override the default models at runtime:

```bash
OLLAMA_EMBED_MODEL=all-minilm \
OLLAMA_CHAT_MODEL=qwen3.5:27b \
docker compose run --rm app python3 /workspace/demo.py
```
