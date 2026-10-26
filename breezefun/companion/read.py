"""Stream an exported Breeze voice, retaining the model across stdin lines."""
import argparse
import hashlib
import importlib.metadata
import json
import math
import os
import platform
import secrets
import sys
import time
import uuid
from contextlib import redirect_stdout
from dataclasses import asdict, dataclass
from datetime import UTC, datetime
from pathlib import Path

from playback import Playback, PlaybackError


@dataclass(frozen=True)
class Settings:
    temperature: float = 0.9
    top_p: float = 1.0
    top_k: int = 50
    repetition_penalty: float = 1.0
    max_tokens: int = 750
    instruction: str = ''
    cfg_scale: float = 1.5
    streaming_interval: float = 0.5
    gain_db: float = 12.0

    def validate(self):
        for name in ('temperature', 'top_p', 'repetition_penalty', 'cfg_scale', 'streaming_interval', 'gain_db'):
            if not math.isfinite(getattr(self, name)):
                raise ValueError(f'{name} must be finite')
        if self.temperature < 0 or not 0 < self.top_p <= 1 or self.top_k < 0:
            raise ValueError('Require temperature >= 0, 0 < top_p <= 1, and top_k >= 0')
        if self.repetition_penalty <= 0 or self.max_tokens <= 0 or self.streaming_interval <= 0:
            raise ValueError('Repetition penalty, max tokens, and streaming interval must be positive')
        if not -60 <= self.gain_db <= 24:
            raise ValueError('Gain must be between -60 and +24 dB')


def hash_file(path):
    digest = hashlib.sha256()
    with Path(path).open('rb') as source:
        for block in iter(lambda: source.read(1024 * 1024), b''):
            digest.update(block)
    return digest.hexdigest()


def voice_context(directory):
    directory = directory.resolve()
    reference = directory/'reference.wav'
    transcript = json.loads((directory/'voice.json').read_text())['transcript']
    if not isinstance(transcript, str) or not transcript.strip():
        raise ValueError('voice.json must contain the exact reference transcript')
    return {'directory': str(directory), 'reference_sha256': hash_file(reference), 'transcript': transcript}


def model_context(directory):
    directory = directory.resolve()
    metadata = directory/'.cache/huggingface/download'
    weights = []
    for path in sorted(directory.rglob('*.safetensors')):
        relative = path.relative_to(directory).as_posix()
        info = metadata/(relative + '.metadata')
        lines = info.read_text().splitlines() if info.is_file() else []
        # HF's LFS ETag is the SHA-256. Reuse downloader metadata rather than
        # rehash gigabytes on launch; hash local files without that metadata.
        etag = lines[1] if len(lines) > 1 else ''
        try:
            metadata_is_current = len(lines) > 2 and path.stat().st_mtime <= float(lines[2])
        except ValueError:
            metadata_is_current = False
        digest = etag if metadata_is_current and len(etag) == 64 and all(c in '0123456789abcdef' for c in etag) else hash_file(path)
        weights.append({'path': relative, 'bytes': path.stat().st_size, 'sha256': digest})
    if not weights:
        raise ValueError('Model directory contains no safetensors weights')
    info = metadata/'config.json.metadata'
    revision = info.read_text().splitlines()[0] if info.is_file() else None
    support_files = [
        {'path': path.relative_to(directory).as_posix(), 'sha256': hash_file(path)}
        for path in sorted(directory.rglob('*'))
        if path.is_file() and '.cache' not in path.relative_to(directory).parts
        and path.suffix in ('.json', '.model', '.vocab', '.txt')
    ]
    return {'directory': str(directory), 'revision': revision,
            'identity': {'config_sha256': hash_file(directory/'config.json'),
                         'weights': weights, 'support_files': support_files}}


def runtime_context(mx):
    packages = ('mlx', 'mlx-metal', 'mlx-audio', 'numpy', 'soundfile', 'sounddevice')
    return {'runner_sha256': hash_file(__file__),
            'python': platform.python_version(), 'macos': platform.mac_ver()[0],
            'device': mx.device_info()['device_name'],
            'packages': {name: importlib.metadata.version(name) for name in packages}}


def amplified_audio(samples, gain_db):
    """Fixed gain with a continuous soft knee, independent of chunk boundaries."""
    import numpy as np

    samples = np.asarray(samples, dtype=np.float32).reshape(-1)
    if not np.isfinite(samples).all():
        raise ValueError('Model returned non-finite audio samples')
    amplified = samples * (10 ** (gain_db / 20))
    magnitude = np.abs(amplified)
    knee, ceiling = 0.85, 0.98
    excess = np.maximum(magnitude - knee, 0)
    limited = np.sign(amplified) * (knee + (ceiling-knee) * np.tanh(excess / (ceiling-knee)))
    return np.where(magnitude <= knee, amplified, limited).astype(np.float32), int(np.count_nonzero(magnitude > knee))


def read_utterance(model, player, text, settings, seed, output_dir, context):
    import soundfile as sf

    ident = uuid.uuid4().hex
    output_dir.mkdir(parents=True, exist_ok=True)
    receipt_path = output_dir/(ident + '.json')
    audio_path = output_dir/(ident + '.wav')
    request = {'text': text, 'seed': seed, **asdict(settings)}
    record = {'format': 'breeze-read-v1', 'id': ident, 'status': 'started',
              'created_at': datetime.now(UTC).isoformat(),
              'request': request, 'limiter': {'kind': 'soft-knee-v1', 'knee': 0.85, 'ceiling': 0.98}, **context}
    receipt_path.write_text(json.dumps(record, indent=2) + '\n')
    started = time.perf_counter()
    first_chunk_seconds = None
    samples_written = limited_samples = chunks = 0
    sample_rate = model.sample_rate
    print(f'Read {ident}: seed={seed}, temperature={settings.temperature}, gain={settings.gain_db:+g} dB', file=sys.stderr)
    try:
        # Breeze creates fresh KV/decoder state in generate. Explicit per-read
        # seeds make requests independent of previous lines' random draws.
        with sf.SoundFile(audio_path, mode='w', samplerate=sample_rate, channels=1, subtype='PCM_16') as output:
            for result in model.generate(
                text=text, voice='S0', ref_audio=str(Path(context['voice']['directory'])/'reference.wav'),
                ref_text=context['voice']['transcript'], instruct=settings.instruction or None,
                cfg_scale=settings.cfg_scale, temperature=settings.temperature,
                top_p=settings.top_p, top_k=settings.top_k, repetition_penalty=settings.repetition_penalty,
                max_tokens=settings.max_tokens, seed=seed, stream=True,
                streaming_interval=settings.streaming_interval,
            ):
                if result.sample_rate != sample_rate:
                    raise ValueError('Model changed sample rate during generation')
                audio, limited = amplified_audio(result.audio, settings.gain_db)
                if not len(audio):
                    continue
                if first_chunk_seconds is None:
                    first_chunk_seconds = time.perf_counter() - started
                chunks += 1
                samples_written += len(audio)
                limited_samples += limited
                output.write(audio)
                if player is not None:
                    player.queue_audio(audio)
        if not samples_written:
            raise ValueError('Breeze returned no audio')
        generation_seconds = time.perf_counter() - started
        # Completion means synthesis/saving finished, not that the speakers
        # drained. All reads share one FIFO so the next can synthesize now.
        record.update(status='complete', playback='queued' if player is not None else 'disabled',
                      audio={'path': str(audio_path.resolve()),
                      'sha256': hash_file(audio_path), 'sample_rate': sample_rate,
                      'seconds': samples_written/sample_rate, 'chunks': chunks, 'limited_samples': limited_samples},
                      timing={'first_chunk_seconds': first_chunk_seconds, 'generation_seconds': generation_seconds})
    except BaseException as error:
        record.update(status='error', error=f'{type(error).__name__}: {error}')
        raise
    finally:
        receipt_path.write_text(json.dumps(record, indent=2) + '\n')
    print(f'Saved {receipt_path}', file=sys.stderr)
    return {'receipt': str(receipt_path.resolve()), **record}


def parse_args(argv=None):
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument('text', nargs='?', help='One utterance; use --stdin for one per line')
    parser.add_argument('--stdin', action='store_true', help='Keep model loaded; process nonblank stdin lines in order')
    parser.add_argument('--replay', type=Path, help='Replay one saved read JSON, including its seed and settings')
    parser.add_argument('--voice-dir', type=Path, help='Voice export directory; default: cwd')
    parser.add_argument('--model', type=Path, help='Local checkpoint; default: BREEZE_MODEL or project checkpoint')
    parser.add_argument('--output-dir', type=Path, default=Path('reads'), help='WAV and JSON receipts; default: reads/')
    parser.add_argument('--no-play', action='store_true', help='Save streamed audio without opening a speaker device')
    parser.add_argument('--buffer-seconds', type=float, default=5.0,
                        help='Maximum queued playback audio in seconds; default: 5')
    parser.add_argument('--seed', type=int, help='Reset this seed per utterance; default: choose and log a new seed')
    for name, field in Settings.__dataclass_fields__.items():
        parser.add_argument('--' + name.replace('_', '-'), type=type(field.default), default=None,
                            help=f'Default: {field.default!r}' + (' seconds of audio per chunk' if name == 'streaming_interval' else ''))
    args = parser.parse_args(argv)
    if not math.isfinite(args.buffer_seconds) or args.buffer_seconds <= 0:
        parser.error('Playback buffer duration must be finite and positive')
    if sum((args.text is not None, args.stdin, args.replay is not None)) != 1:
        parser.error('Provide text, --stdin, or --replay (exactly one)')
    if args.seed is not None and not 0 <= args.seed < 2**32:
        parser.error('Seed must be between 0 and 4294967295')
    if args.text is not None and not args.text.strip():
        parser.error('Text must not be blank')
    if args.replay and (args.seed is not None or any(getattr(args, name) is not None for name in Settings.__dataclass_fields__)):
        parser.error('--replay uses saved seed/settings; only paths and playback options may be overridden')
    return args


def main(argv=None):
    args = parse_args(argv)
    replay = json.loads(args.replay.read_text()) if args.replay else None
    if replay and replay.get('format') != 'breeze-read-v1':
        raise ValueError('Unsupported read receipt format')
    default_model = Path(__file__).resolve().parent.parent/'.models/breeze-tts-2-8bit'
    model_dir = args.model or Path(os.environ.get('BREEZE_MODEL') or (replay['model']['directory'] if replay else default_model))
    voice_dir = args.voice_dir or Path(replay['voice']['directory'] if replay else Path.cwd())
    values = {name: getattr(args, name) for name in Settings.__dataclass_fields__ if getattr(args, name) is not None}
    if 'cfg_scale' not in values and values.get('instruction'):
        values['cfg_scale'] = 4.0
    request = replay['request'] if replay else None
    settings = Settings(**({name: request[name] for name in Settings.__dataclass_fields__} if replay else values))
    settings.validate()
    voice = voice_context(voice_dir)
    checkpoint = model_context(model_dir)
    if replay:
        if (voice['reference_sha256'], voice['transcript']) != (replay['voice']['reference_sha256'], replay['voice']['transcript']):
            raise ValueError('Reference audio or transcript differs from the saved read')
        if checkpoint['identity'] != replay['model']['identity']:
            raise ValueError('Checkpoint differs from the saved read')
    import mlx.core as mx
    from mlx_audio.tts import load

    print('Loading Breeze once…', file=sys.stderr)
    with redirect_stdout(sys.stderr):
        model = load(str(model_dir.resolve()))
    context = {'voice': voice, 'model': checkpoint, 'runtime': runtime_context(mx)}
    if replay and context['runtime'] != replay['runtime']:
        print('Runtime differs from saved read; identical audio is not guaranteed.', file=sys.stderr)
    if args.stdin:
        print('Ready. One utterance per line; Ctrl-D to finish. Model stays loaded.', file=sys.stderr)
    lines = sys.stdin if args.stdin else [request['text'] if replay else args.text]
    failed = False
    player = None if args.no_play else Playback(model.sample_rate, args.buffer_seconds)
    try:
        for line in lines:
            text = line.strip()
            if not text:
                continue
            seed = request['seed'] if replay else (args.seed if args.seed is not None else secrets.randbits(32))
            try:
                with redirect_stdout(sys.stderr):
                    result = read_utterance(model, player, text, settings, seed, args.output_dir, context)
                print(json.dumps({'receipt': result['receipt'], 'seed': seed, 'audio': result['audio'],
                                  'playback': result['playback']}), flush=True)
            except PlaybackError:
                raise
            except Exception as error:
                print(f'Read failed: {error}', file=sys.stderr)
                if not args.stdin:
                    raise
                failed = True
        if player is not None:
            player.stop()
    except BaseException:
        if player is not None:
            player.flush()
        raise
    return int(failed)


if __name__ == '__main__':
    try:
        sys.exit(main())
    except KeyboardInterrupt:
        sys.exit(130)
