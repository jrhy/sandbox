"""One bounded FIFO and one owner thread for streaming speaker output."""
import math
from collections import deque
from threading import Condition, Thread


class PlaybackError(RuntimeError):
    """Speaker output failed or was stopped while a producer was active."""


def open_output_stream(**kwargs):
    import sounddevice as sd

    return sd.OutputStream(**kwargs)


class Playback:
    def __init__(self, sample_rate, buffer_seconds=5.0, stream_factory=open_output_stream):
        if sample_rate <= 0 or not math.isfinite(buffer_seconds) or buffer_seconds <= 0:
            raise ValueError('Sample rate and playback buffer duration must be positive')
        self.sample_rate = sample_rate
        self.capacity_frames = max(1, int(sample_rate * buffer_seconds))
        self.block_frames = min(2048, self.capacity_frames)
        self._stream_factory = stream_factory
        self._condition = Condition()
        self._pending = deque()
        self._buffered_frames = 0  # Includes the block currently passed to write().
        self._closing = False
        self._cancelled = False
        self._error = None
        self._thread = None

    @property
    def buffered_frames(self):
        with self._condition:
            return self._buffered_frames

    def _check_error(self):
        if self._error is not None:
            raise PlaybackError('Audio playback failed') from self._error
        if self._cancelled or self._closing:
            raise PlaybackError('Audio playback has stopped')

    def queue_audio(self, samples):
        import numpy as np

        samples = np.ascontiguousarray(samples, dtype=np.float32).reshape(-1)
        for offset in range(0, len(samples), self.block_frames):
            block = samples[offset:offset + self.block_frames]
            with self._condition:
                self._check_error()
                if self._thread is None:
                    self._thread = Thread(target=self._play, name='breeze-playback')
                    self._thread.start()
                while self._buffered_frames + len(block) > self.capacity_frames:
                    self._condition.wait()
                    self._check_error()
                self._pending.append(block)
                self._buffered_frames += len(block)
                self._condition.notify_all()

    def _play(self):
        stream = None
        try:
            # Only this thread touches PortAudio. MLX stays on the main thread.
            stream = self._stream_factory(samplerate=self.sample_rate, channels=1, dtype='float32')
            stream.start()
            while True:
                with self._condition:
                    while not self._pending and not self._closing and not self._cancelled:
                        self._condition.wait()
                    if self._cancelled or not self._pending:
                        break
                    block = self._pending.popleft()
                # Blocking writes pace playback on this worker, never on the
                # synthesis thread unless the bounded queue is full.
                stream.write(block)
                with self._condition:
                    self._buffered_frames -= len(block)
                    self._condition.notify_all()
        except Exception as error:  # noqa: BLE001 -- propagate worker failures to the producer
            with self._condition:
                self._error = error
                self._cancelled = True
                self._condition.notify_all()
        finally:
            if stream is not None:
                try:
                    # stop() drains the device; close() alone would discard
                    # pending audio. Cancellation instead uses abort().
                    if self._cancelled:
                        stream.abort()
                    else:
                        stream.stop()
                except Exception as error:  # noqa: BLE001 -- propagate worker failures to the producer
                    with self._condition:
                        self._error = self._error or error
                finally:
                    try:
                        stream.close()
                    except Exception as error:  # noqa: BLE001 -- propagate worker failures to the producer
                        with self._condition:
                            self._error = self._error or error
            with self._condition:
                self._pending.clear()
                self._buffered_frames = 0
                self._condition.notify_all()

    def stop(self):
        """Finish all queued speech and device buffers, then join the worker."""
        with self._condition:
            self._closing = True
            self._condition.notify_all()
        if self._thread is not None:
            self._thread.join()
        if self._error is not None:
            raise PlaybackError('Audio playback failed') from self._error

    def flush(self):
        """Discard queued speech and join after the current short device write."""
        with self._condition:
            self._cancelled = True
            self._pending.clear()
            self._condition.notify_all()
        if self._thread is not None:
            self._thread.join()
