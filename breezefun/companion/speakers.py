"""Conservative speaker selection and reference assembly, independent of MLX."""
import wave
from collections import defaultdict


def clean_segments(segments, duration, guard_seconds=0.12, min_seconds=1.0):
    """Keep only intervals with exactly one predicted speaker, then trim edges.

    A sweep over all boundaries removes overlaps rather than choosing a winner.
    Adjacent intervals for the same speaker are joined before edge trimming.
    """
    events = defaultdict(lambda: defaultdict(int))
    for segment in segments:
        start = max(0.0, min(duration, float(segment['start'])))
        end = max(0.0, min(duration, float(segment['end'])))
        speaker = int(segment['speaker'])
        if end <= start or speaker not in range(4):
            continue
        events[start][speaker] += 1
        events[end][speaker] -= 1
    active = defaultdict(int)
    previous = 0.0
    clean = []
    for timestamp in sorted(events):
        speakers = [speaker for speaker, count in active.items() if count > 0]
        if len(speakers) == 1 and timestamp > previous:
            speaker = speakers[0]
            # Chunk timestamps can differ by a floating-point rounding error.
            if clean and clean[-1]['speaker'] == speaker and abs(clean[-1]['end'] - previous) < 1e-6:
                clean[-1]['end'] = timestamp
            else:
                clean.append({'speaker': speaker, 'start': previous, 'end': timestamp})
        for speaker, change in events[timestamp].items():
            active[speaker] += change
        previous = timestamp
    result = []
    for segment in clean:
        start = segment['start'] + guard_seconds
        end = segment['end'] - guard_seconds
        if end - start >= min_seconds:
            result.append({'speaker': segment['speaker'], 'start': start, 'end': end})
    return result


def assemble_reference(source, destination, segments, max_seconds, gap_seconds=0.12):
    """Concatenate chronological speech with short silent joins, capped in seconds."""
    used = []
    with wave.open(str(source), 'rb') as reader:
        if reader.getnchannels() != 1 or reader.getsampwidth() != 2:
            raise ValueError('Expected normalized mono PCM16 source audio.')
        sample_rate = reader.getframerate()
        budget_frames = int(max_seconds * sample_rate)
        gap_frames = int(gap_seconds * sample_rate)
        written_frames = 0
        with wave.open(str(destination), 'wb') as writer:
            writer.setparams(reader.getparams())
            for segment in sorted(segments, key=lambda s: s['start']):
                gap = gap_frames if used else 0
                remaining = budget_frames - written_frames - gap
                if remaining < sample_rate:
                    break
                start_frame = round(segment['start'] * sample_rate)
                frames = min(round(segment['end'] * sample_rate) - start_frame, remaining)
                if frames < sample_rate:
                    continue
                reader.setpos(start_frame)
                audio = reader.readframes(frames)
                if used:
                    writer.writeframes(b'\0\0' * gap)
                writer.writeframes(audio)
                written_frames += gap + len(audio) // 2
                used.append({**segment, 'end': (start_frame + len(audio) // 2) / sample_rate})
    if not used:
        raise ValueError('No selected segments contain at least one second of speech.')
    return used, written_frames / sample_rate
