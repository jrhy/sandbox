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

## Louder streaming reads and a resident stdin reader

From an unzipped voice-export directory (containing `voice.json` and
`reference.wav`), run the local reader:

```sh
"$HOME/sandbox/breezefun/companion/.venv/bin/python" \
  "$HOME/sandbox/breezefun/companion/read.py" \
  --stdin --gain-db 12 --seed 42
```

It loads Breeze once, then synthesizes each nonblank stdin line in order. One
speaker-output worker plays a shared FIFO while the main thread generates
audio. Once a line's synthesis finishes, the next available line starts without
waiting for that audio to finish playing. Speech stays in order and voices do
not overlap. Short utterances start playing without waiting for another line.
Ctrl-D drains all queued speech before exit; Ctrl-C discards pending playback
after the current short device write. Each utterance has
fresh generation state and its seed is reset independently. The model weights
stay loaded, but the reference prompt is still encoded for each request.

The same reader works inside the sandbox:

```sh
sb exec --gpu --allow-read ~/.local --allow-read ~/sandbox/breezefun \
  "$BREEZE_PYTHON" ~/sandbox/breezefun/companion/read.py \
  --stdin --gain-db 12 --seed 42
```

For one read, replace `--stdin` with quoted text. `--voice-dir DIR` selects a
different export, `--model DIR` overrides `BREEZE_MODEL` or the project-local
checkpoint, and `--no-play` generates files without opening a speaker device.
The companion server does not need to be running.

`--buffer-seconds 5` caps the queue at five seconds of audio, including the
block currently being submitted to the device. When full, synthesis waits for
playback to make room. This bounds memory for long pipelines while allowing
synthesis to work ahead. Increase it to allow a larger backlog. This is a
queue limit, not an initial playback delay. A later line cannot start before
the current line's synthesis finishes, and gaps remain possible if generation
is slower than speech. The model is used only on the main thread; the output
worker handles NumPy audio and PortAudio, without touching MLX or its RNG.

Gain is applied to both playback and saved audio. +12 dB is about four times the
sample amplitude. A soft limiter begins at 0.85 and bounds peaks at 0.98; it
does not normalize each chunk, so chunk boundaries do not change the gain.
`--gain-db 0` disables the boost; the accepted range is -60 to +24 dB. Your
system speaker volume remains in control of the device output level.

Every read saves `reads/<id>.wav` and `reads/<id>.json`, or files under
`--output-dir DIR`. The WAV preserves the exact read. Its JSON receipt records
the text, actual seed, temperature, top-p/top-k, repetition penalty, token limit,
instruction, CFG scale, chunk interval, gain and limiter settings, original
reference transcript and audio hash, model fingerprints/revision, runtime
versions, runner hash, timings, and output hash. Failed generation leaves an
error receipt and stdin mode continues with the next line, preserving speech
already queued. Device failures are surfaced to the main thread on enqueue or
drain. Diagnostics go to stderr; stdout emits a JSON completion summary for
each successful synthesis/save. `playback: "queued"` means the audio was
submitted for ordered playback, not that it has finished speaking. The process
waits for final device buffers before normal exit. Queue backpressure is
included in the logged generation wall time.
The default `reads/` directory is ignored by Git.

Replay a saved read's generation settings:

```sh
"$BREEZE_PYTHON" ~/sandbox/breezefun/companion/read.py \
  --replay reads/READ_ID.json
```

Replay verifies the reference and checkpoint identity before generation. Use
`--voice-dir` and `--model` to relocate the same inputs on another machine.
It warns if runtime versions differ. Model fingerprints reuse current Hugging
Face download checksums; local or modified weights without current metadata
are hashed at startup. Tokenizer/configuration files are also fingerprinted.

With `--seed 42`, the same seed is reset for every line. Omit `--seed` for a new
random seed on each read; that actual seed is always saved for replay. The
default `--temperature 0.9` samples; `--temperature 0` uses greedy selection.
Other controls are `--top-p`, `--top-k`, `--repetition-penalty`, `--max-tokens`,
`--instruction`, `--cfg-scale`, and `--streaming-interval` (seconds of audio per
chunk, default 0.5). Instructions default CFG scale to 4; otherwise it is 1.5.

Same-seed reads were verified to produce byte-identical WAVs on this M2 Pro
after an intervening read, and replay was tested in a fresh process. This is
not a guarantee across different GPU hardware, OS/runtime versions, model
weights, or reference inputs. Keep the saved WAV when the exact delivery matters.
The browser companion and existing exported `speak.py` keep their original
behavior; these options belong to the separate local `read.py` runner.

Playback scheduling is covered by event-gated tests for generation overlapping
a preceding audio write, FIFO ordering, bounded backlog (including in-flight
samples), short utterances, EOF drain, cancellation, and device failures. Two
hundred repeated scheduling/cancellation checks passed without leftover worker
threads. A live sandboxed run verified queued playback and same-seed WAV identity.

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
