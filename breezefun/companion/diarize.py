"""Run NVIDIA Sortformer in its own MLX process, emitting JSON on the last line."""
import json
import os
import sys


def main():
    from mlx_audio.vad import load

    model = load(os.environ['SORTFORMER_MODEL'])
    segments = []
    # Seconds, per the MLX Sortformer generate_stream contract. Streaming keeps
    # attention bounded and retains its speaker cache across the whole source.
    chunk_duration_seconds = 5.0
    for result in model.generate_stream(
        sys.argv[1], chunk_duration=chunk_duration_seconds,
        threshold=0.5, min_duration=0.0, merge_gap=0.0,
    ):
        segments.extend({
            'start': segment.start, 'end': segment.end, 'speaker': segment.speaker,
        } for segment in result.segments)
    print(json.dumps({'segments': segments}))


if __name__ == '__main__':
    main()
