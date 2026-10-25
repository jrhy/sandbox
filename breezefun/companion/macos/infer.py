"""Adapt the companion's official Breeze CLI arguments to MLX Audio on a Mac."""
import argparse
from pathlib import Path


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument('model')
    parser.add_argument('--ref-audio', required=True)
    parser.add_argument('--ref-text', required=True)
    parser.add_argument('--text', required=True)
    parser.add_argument('--output', required=True)
    parser.add_argument('--instruction', default='')
    parser.add_argument('--cfg-scale', type=float, default=1.5)
    args = parser.parse_args()

    import mlx.core as mx
    import numpy as np
    import soundfile as sf
    from mlx_audio.tts import load

    model = load(args.model)
    results = list(model.generate(
        text=args.text,
        voice='S0',
        ref_audio=args.ref_audio,
        ref_text=args.ref_text,
        instruct=args.instruction or None,
        cfg_scale=args.cfg_scale,
    ))
    if not results:
        raise RuntimeError('Breeze returned no audio.')
    audio = mx.concatenate([result.audio for result in results])
    Path(args.output).parent.mkdir(parents=True, exist_ok=True)
    sf.write(args.output, np.asarray(audio), results[0].sample_rate)


if __name__ == '__main__':
    main()
