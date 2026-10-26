"""Event-gated playback scheduling tests: no audio device, GPU, or sleeps."""
import threading
import unittest
from unittest.mock import Mock

import numpy as np

from playback import Playback, PlaybackError


class GatedStream:
    def __init__(self, gated=False, fail_write=False):
        self.entered = threading.Event()
        self.release = threading.Event()
        self.finished_first = threading.Event()
        self.gated = gated
        self.fail_write = fail_write
        self.blocks = []
        self.calls = []

    def start(self):
        self.calls.append(('start', threading.get_ident()))

    def write(self, block):
        self.calls.append(('write', threading.get_ident()))
        if not self.blocks:
            self.entered.set()
            if self.gated:
                timeout_seconds = 3
                if not self.release.wait(timeout_seconds):
                    raise TimeoutError('Test did not release the first device write')
            if self.fail_write:
                raise RuntimeError('Device disconnected')
            self.finished_first.set()
        self.blocks.append(block.copy())

    def stop(self):
        self.calls.append(('stop', threading.get_ident()))

    def abort(self):
        self.calls.append(('abort', threading.get_ident()))

    def close(self):
        self.calls.append(('close', threading.get_ident()))


class PlaybackTests(unittest.TestCase):
    def wait(self, event):
        timeout_seconds = 3
        self.assertTrue(event.wait(timeout_seconds), 'Scheduling barrier was not reached')

    def join(self, thread):
        timeout_seconds = 3
        thread.join(timeout_seconds)
        self.assertFalse(thread.is_alive(), 'Thread did not finish')

    def player(self, stream, capacity=4):
        factory = Mock(return_value=stream)
        player = Playback(sample_rate=4, buffer_seconds=capacity/4, stream_factory=factory)
        # A failed assertion must not leave the device or a producer parked.
        self.addCleanup(player.flush)
        self.addCleanup(stream.release.set)
        return player, factory

    def blocked_producer(self, player, audio):
        waiting, done = threading.Event(), threading.Event()
        errors = []
        original_wait = player._condition.wait

        def observe_wait(timeout=None):
            if threading.current_thread() is producer:
                waiting.set()
            return original_wait(timeout)

        player._condition.wait = observe_wait

        def enqueue():
            try:
                player.queue_audio(audio)
            except PlaybackError as error:
                errors.append(error)
            finally:
                done.set()

        producer = threading.Thread(target=enqueue, name='test-producer')
        producer.start()
        self.wait(waiting)
        return producer, done, errors

    def test_backpressure_counts_inflight_audio_and_preserves_order(self):
        stream = GatedStream(gated=True)
        player, factory = self.player(stream)
        player.queue_audio(np.ones(4))
        self.wait(stream.entered)
        producer, done, errors = self.blocked_producer(player, np.full(4, 2))
        self.assertEqual(player.buffered_frames, 4)
        self.assertFalse(done.is_set())
        stream.release.set()
        self.join(producer)
        player.stop()
        self.assertEqual(errors, [])
        np.testing.assert_array_equal(np.concatenate(stream.blocks), [1]*4 + [2]*4)
        self.assertEqual(player.buffered_frames, 0)
        factory.assert_called_once()
        self.assertEqual([name for name, _ in stream.calls][-2:], ['stop', 'close'])
        self.assertEqual(len({ident for _, ident in stream.calls}), 1)
        self.assertNotEqual(stream.calls[0][1], threading.get_ident())

    def test_fifo_spans_chunks_larger_than_capacity(self):
        stream = GatedStream()
        player, _ = self.player(stream, capacity=3)
        player.queue_audio(np.arange(17, dtype=np.float32))
        player.stop()
        np.testing.assert_array_equal(np.concatenate(stream.blocks), np.arange(17))
        self.assertLessEqual(max(len(block) for block in stream.blocks), 3)

    def test_short_read_starts_without_waiting_for_more_input(self):
        stream = GatedStream()
        player, _ = self.player(stream)
        player.queue_audio([.25])
        self.wait(stream.entered)
        player.stop()
        np.testing.assert_array_equal(np.concatenate(stream.blocks), [.25])
        self.assertEqual([name for name, _ in stream.calls][-2:], ['stop', 'close'])

    def test_cancellation_wakes_blocked_producer_and_discards_backlog(self):
        stream = GatedStream(gated=True)
        player, _ = self.player(stream)
        player.queue_audio(np.ones(4))
        self.wait(stream.entered)
        producer, done, errors = self.blocked_producer(player, np.full(4, 2))
        flusher = threading.Thread(target=player.flush)
        flusher.start()
        self.wait(done)  # Wakes before the current device write finishes.
        self.assertIsInstance(errors[0], PlaybackError)
        stream.release.set()
        self.join(producer)
        self.join(flusher)
        self.assertEqual(len(stream.blocks), 1)
        self.assertEqual([name for name, _ in stream.calls][-2:], ['abort', 'close'])

    def test_device_failure_wakes_blocked_producer_and_surfaces_on_drain(self):
        stream = GatedStream(gated=True, fail_write=True)
        player, _ = self.player(stream)
        player.queue_audio(np.ones(4))
        self.wait(stream.entered)
        producer, _, errors = self.blocked_producer(player, np.full(4, 2))
        stream.release.set()
        self.join(producer)
        self.assertIsInstance(errors[0], PlaybackError)
        with self.assertRaises(PlaybackError) as caught:
            player.stop()
        self.assertIn('Device disconnected', str(caught.exception.__cause__))
        self.assertEqual([name for name, _ in stream.calls][-2:], ['abort', 'close'])

    def test_no_audio_does_not_open_device(self):
        factory = Mock()
        player = Playback(24000, stream_factory=factory)
        player.stop()
        factory.assert_not_called()
        with self.assertRaises(PlaybackError):
            player.queue_audio([.1])

    def test_device_open_failure_is_reported_and_joined(self):
        player = Playback(24000, stream_factory=Mock(side_effect=RuntimeError('No device')))
        player.queue_audio([.1])
        with self.assertRaisesRegex(PlaybackError, 'Audio playback failed'):
            player.stop()
        self.assertFalse(player._thread.is_alive())
        player.flush()


if __name__ == '__main__':
    unittest.main()
