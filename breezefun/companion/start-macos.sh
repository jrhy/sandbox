#!/bin/sh
set -eu
companion_dir=$(CDPATH= cd -- "$(dirname -- "$0")" && pwd)
project_dir=$(dirname "$companion_dir")
export PATH="$companion_dir/.venv/bin:/opt/homebrew/bin:$PATH"
export HF_HOME="$project_dir/.models/huggingface"
export HF_HUB_CACHE="$HF_HOME/hub"
export HF_XET_CACHE="$HF_HOME/xet"
export BREEZE_REPO="$companion_dir/macos"
export BREEZE_MODEL="$project_dir/.models/breeze-tts-2-8bit"
export BREEZE_PYTHON="$companion_dir/.venv/bin/python"
export PARAKEET_PYTHON="$BREEZE_PYTHON"
export PARAKEET_BACKEND=mlx
export PARAKEET_MODEL="$project_dir/.models/parakeet-tdt-0.6b-v3"
export SORTFORMER_MODEL="$project_dir/.models/sortformer-4spk-v1"
exec "$BREEZE_PYTHON" "$companion_dir/server.py"
