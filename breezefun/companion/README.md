# Breeze Voice Studio companion

This companion runs the real processing pipeline. The hosted studio is a frontend, not a GPU inference service. No model weights are bundled. Inference has not been validated on GPU in the build environment.

## Requirements

- Official Breeze TTS 2: Linux, Python 3.10+, CUDA-capable NVIDIA GPU. Upstream recommends 12 GB GPU memory for eager inference (roughly 7.7 GiB used). Allow more capacity if keeping other models loaded. This companion runs Parakeet in a separate process and releases it before Breeze starts.
- ffmpeg and yt-dlp for extraction and decoding.
- NVIDIA Parakeet TDT 0.6B v3, installed through NeMo. Its first transcription downloads model weights.
- Enough disk space for both models and source audio. Sources are limited to two hours, uploads to 100 MB, and reference clips to 1–30 seconds. Keep reference audio shorter and cleaner where possible.

## 1. Install the companion

Unzip this archive and open a terminal in its directory:

```sh
python3 -m venv .venv
. .venv/bin/activate
python -m pip install -r requirements.txt
```

Install ffmpeg using your Linux distribution's package manager.

## 2. Install the model environments

Use separate virtual environments to avoid dependency conflicts. Follow the current upstream installation instructions:

- Breeze runtime: https://github.com/breezeblue-ai/breeze-tts
- Breeze checkpoint: https://huggingface.co/BreezeBlue/Breeze-TTS-2
- Parakeet / NeMo: https://huggingface.co/nvidia/parakeet-tdt-0.6b-v3

Example Parakeet environment (follow NVIDIA's instructions if platform requirements differ):

```sh
python3 -m venv parakeet-env
. parakeet-env/bin/activate
python -m pip install 'nemo_toolkit[asr]'
```

Clone the official Breeze repository, install its requirements in its own environment, and download the full checkpoint into a directory. Set absolute paths below. No multi-gigabyte download or GPU rental is started by this website.

## 3. Start the companion

Reactivate the companion environment and set paths to the installed environments:

```sh
. .venv/bin/activate
export BREEZE_REPO=/absolute/path/to/breeze-tts
export BREEZE_MODEL=/absolute/path/to/breeze-tts-2
export BREEZE_PYTHON=/absolute/path/to/breeze-env/bin/python
export PARAKEET_PYTHON=/absolute/path/to/parakeet-env/bin/python
python server.py
```

The server binds to 127.0.0.1:8765. It prints a random access token. Open http://127.0.0.1:8765 on that same computer, select Connect companion and enter the token. The token is saved in this browser's localStorage for that companion address, and the studio reconnects on reload. A rejected saved token is cleared so you can enter a replacement. Each server restart generates a new token unless VOICE_TOKEN is set in the environment.

The hosted studio may connect to localhost if your browser permits it. If local-network or mixed-content protections block that connection, use the bundled local studio. On a phone, localhost refers to the phone, not a separate GPU computer. For remote GPU use, place this loopback server behind an authenticated HTTPS reverse proxy or private tunnel. Do not expose it directly on the public internet. Set STUDIO_ORIGIN to the exact hosted frontend origin if using a different Site.

The connection check only checks configured paths and local utilities. It does not validate GPU memory, weight compatibility, or successful model loading. Those checks happen on first transcription/synthesis. Model commands can take minutes and logs/errors are returned to the studio.

## Workflow

1. Paste a YouTube URL and preview it. Extract audio through the companion, or upload a file.
2. Set clip start/end in seconds; use the down arrows to mark the current audio playback time. Preview the cut.
3. Confirm the speaker has permitted this use, then create the clip and transcribe.
4. Listen to the reference and correct every spoken word in the transcript.
5. Enter new text, optionally set a delivery instruction, and generate. Play/download the resulting WAV.
6. Export the voice pack. It contains reference.wav, voice.json, speak.py and usage notes. It is a reusable reference, not a newly trained model or a universal voice file.

YouTube can block server downloads, require authentication, restrict a video, or change extraction behavior. This companion does not handle YouTube credentials. Update yt-dlp or upload an authorized audio file instead. Embedded YouTube playback may itself be unavailable for some videos. Parakeet v3 supports English and 24 other European languages; Breeze supports English and Mandarin. Automatic Mandarin transcription is not supported by this Parakeet model. For Mandarin references, replace the returned transcript manually. If Parakeet fails, the reference clip remains usable and the studio asks for a manual transcript.

Audio and source metadata are stored under data/ on the companion host (or VOICE_DATA). They are not uploaded to the Site. Browser tabs hold audio for playback and download. Source and voice files persist on the host until you delete them; job status and browser workflow state are session-only. Stop the companion before deleting its data directory. Export voices before clearing data. New synthesis requests do not overwrite old voice packs.

## CLI use after export

Unzip the exported voice pack. Activate your Breeze environment or set BREEZE_PYTHON, then:

```sh
export BREEZE_REPO=/absolute/path/to/breeze-tts
export BREEZE_MODEL=/absolute/path/to/breeze-tts-2
python speak.py 'Hello there.' --output speech.wav
python speak.py 'Time for the next chapter.' --instruction 'Speak warmly and calmly.' --output chapter.wav
```

This uses the official Python CLI flags. It is not a GGUF or ONNX export. Other runtimes may accept the same reference audio/transcript, but their adapters are not implemented here. Breeze loads per request; this conservative first version prioritizes portability over low latency. Parakeet similarly loads per transcription. Audio synthesis was not run during development because no GPU model service was connected.

## License and use

Breeze source is Apache 2.0, but its model weights, derivative models and self-hosted outputs are governed by BreezeBlue's research and non-commercial license. Review the upstream license for your intended use. Parakeet v3 uses CC BY 4.0. Use voices and source recordings with the necessary permissions.
