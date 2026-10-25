#!/bin/sh
# Recreate the local studio on an Apple Silicon Mac. Models remain untracked.
set -eu

start_server=true
case "${1-}" in
    '') ;;
    --no-start) start_server=false ;;
    *) printf 'Usage: %s [--no-start]\n' "$0" >&2; exit 2 ;;
esac
if [ "$#" -gt 1 ]; then
    printf 'Usage: %s [--no-start]\n' "$0" >&2
    exit 2
fi

if [ "$(uname -s)" != Darwin ] || [ "$(uname -m)" != arm64 ]; then
    printf 'This setup requires Apple Silicon macOS (run from a native arm64 shell).\n' >&2
    exit 1
fi

project_dir=$(CDPATH= cd -- "$(dirname -- "$0")" && pwd)
cd "$project_dir"
export PATH="/opt/homebrew/bin:$PATH"

run_step() {
    step_label=$1
    shift
    printf '%s…\n' "$step_label"
    if step_output=$("$@" 2>&1); then
        printf '✓ %s\n' "$step_label"
    else
        printf '%s\n' "$step_output" | tail -40 >&2
        return 1
    fi
}

for dependency in uv ffmpeg; do
    if ! command -v "$dependency" >/dev/null 2>&1; then
        if ! command -v brew >/dev/null 2>&1; then
            printf 'Install Homebrew from https://brew.sh, then run this script again.\n' >&2
            exit 1
        fi
        run_step "Install $dependency" brew install "$dependency"
    fi
done

python_bin="$project_dir/companion/.venv/bin/python"
if [ ! -x "$python_bin" ]; then
    run_step 'Create Python 3.12 environment' uv venv --python 3.12 companion/.venv
fi
run_step 'Check Python version' "$python_bin" -c \
    'import sys; sys.exit(0 if sys.version_info[:2] == (3, 12) else "Existing companion/.venv must use Python 3.12; move it aside and rerun setup.")'
run_step 'Install pinned Python dependencies' uv pip sync --python "$python_bin" requirements-macos.lock

printf 'Download pinned model checkpoints (about 7 GB on first setup)…\n'
# Direct HTTP avoids the accelerated downloader that stalled during initial setup.
HF_HUB_DISABLE_XET=1 HF_HUB_DISABLE_PROGRESS_BARS=1 \
    "$python_bin" companion/download-models.py

if [ "$start_server" = true ]; then
    exec "$project_dir/companion/start-macos.sh"
fi
printf 'Setup complete. Start the studio with: %s/companion/start-macos.sh\n' "$project_dir"
