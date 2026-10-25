# Breeze Voice Studio on this Mac

This directory contains the local companion and studio. The hosted Site has not been changed.
The local companion uses community MLX ports on Apple Silicon instead of CUDA.

On an Apple Silicon Mac, install [Homebrew](https://brew.sh) if needed, then run
from this directory:

```sh
./setup.sh
```

This installs missing `uv` and `ffmpeg` commands, creates a Python 3.12 virtual
environment, synchronizes the pinned dependencies, downloads the three pinned
model checkpoints (about 7 GB), and starts the companion. Use `./setup.sh --no-start`
to prepare the environment without starting another server. It can be rerun to
reuse existing environments and downloads. Dependencies, models, recordings,
and runtime state are ignored by Git.

After setup, start the companion directly:

```sh
./companion/start-macos.sh
```

Open http://127.0.0.1:8765, choose **Connect companion**, and paste the token
printed in that terminal. You can also enter that address and token in the
[hosted studio](https://breeze-voice-studio.johndoe736.chatgpt.site), if your
browser allows access to localhost. The studio remembers the token in localStorage
and reconnects on reload. Tokens are saved separately for each companion address.
Stop with Ctrl-C. Each server restart changes the token unless `VOICE_TOKEN` is set;
a rejected saved token is cleared so you can enter the new one.

For the instance started during setup, the startup token and logs are in
`.runtime/server.log` (readable only by your user). Its local Firefox tab is
already connected. The server listens only on `127.0.0.1`.

Local setup:

- Python 3.12 environment: `companion/.venv`.
- `ffmpeg` installed through Homebrew; `yt-dlp` in the Python environment.
- Breeze: `mlx-community/Breeze-TTS-2-mlx-8bit` in `.models/breeze-tts-2-8bit`.
- Parakeet: `mlx-community/parakeet-tdt-0.6b-v3` in `.models/parakeet-tdt-0.6b-v3`.
- Sortformer: `mlx-community/diar_sortformer_4spk-v1-fp16` in `.models/sortformer-4spk-v1`.
- Audio and voice packs: `companion/data/`. Models, environment, runtime logs,
  and audio are ignored by Git.
- `companion/macos/infer.py` preserves the CLI used by both the companion and
  exported `speak.py`. Run the latter with the same `BREEZE_*` variables from
  `companion/start-macos.sh`.

The models load in separate processes for each operation. Initial generation
can be slow. See [the companion documentation](companion/README.md) for the
workflow and upstream requirements, and [MLX Breeze documentation](https://blaizzy.github.io/mlx-audio/models/tts/breeze-tts/)
for the Mac runtime. Breeze's model license remains research/non-commercial.

To recreate Python dependencies, use `uv venv --python 3.12 companion/.venv`
and `uv pip sync --python companion/.venv/bin/python requirements-macos.lock`.
Download the checkpoints with `companion/.venv/bin/python companion/download-models.py`.

## Reference clips by speaker

In the local studio, upload/import audio or open one of the **Previous recordings**.
Under **Make the cut**, choose **Find speakers**. Select a speaker, preview the
excerpts, and uncheck mistaken assignments. Confirm permission to use the voice,
choose a maximum reference length (default 20 seconds, up to 30), then choose
**Assemble speaker & transcribe**. Listen to the assembled reference and correct
its complete transcript before generating or exporting.

Sortformer runs locally in a separate MLX process. It supports up to four speakers
and keeps speaker state across five-second processing chunks. The companion
removes intervals where multiple speakers are predicted, trims 0.12 seconds from
segment edges, and drops fragments shorter than one second. Selected excerpts are
joined chronologically with 0.12-second silent gaps. The last excerpt may be
shortened to respect the chosen limit. Exported `voice.json` retains original
source timestamps for every included excerpt.

Diarization is an estimate: speaker labels can be wrong and missed overlaps can
remain audible. It does not separate simultaneous voices. Labels are anonymous
and specific to one recording. Analysis is cached in that source's `speakers.json`;
recordings and cached analysis remain local under `companion/data/`.

Verification: seven CPU/integration tests cover overlap removal, stream boundary
rounding, exact selected PCM samples, duration limits, authorization, speaker
selection validation, transcription, and export. A real Sortformer test correctly
grouped two alternating synthetic voices across four turns, producing separate
references of about 13 seconds whose Parakeet transcripts contained their
respective dialogue. Results are in `.runtime/sortformer-smoke-result.json`.

## Verified on this Mac

On 2026-10-25, the companion's three CPU integration tests passed. Adapter lint
and launcher syntax checks passed. All three checkpoint files passed SHA-256
verification against Hugging Face metadata.

A real API smoke test uploaded a synthetic recording, decoded and trimmed it,
transcribed it with MLX Parakeet, cloned it with MLX Breeze using a delivery
instruction, downloaded the generated WAV, and exported a voice pack. Breeze
produced 2.8 seconds of 24 kHz audio. Parakeet transcribed that output as
“Breeze Voice Studio is running locally on this Mac.”

The sample output is `.runtime/smoke-output.wav`; the sample voice pack is
`.runtime/smoke-voice.zip`. Those files and test audio are local and ignored.
