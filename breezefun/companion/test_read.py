"""CPU-only streaming/gain/replay tests; no GPU, speaker, model, or network."""
import io
import json
import os
import sys
import tempfile
import types
import unittest
from contextlib import redirect_stderr, redirect_stdout
from dataclasses import replace
from pathlib import Path
from unittest.mock import Mock, patch

import numpy as np
import soundfile as sf

import read
from playback import Playback
from test_playback import GatedStream


class ReaderTests(unittest.TestCase):
    def setUp(self):
        self.temp = tempfile.TemporaryDirectory()
        self.addCleanup(self.temp.cleanup)
        self.root = Path(self.temp.name)
        self.voice = self.root/'voice'
        self.voice.mkdir()
        sf.write(self.voice/'reference.wav', np.zeros(100), 24000)
        (self.voice/'voice.json').write_text(json.dumps({'transcript': 'Reference words.'}))
        self.model_dir = self.root/'model'
        self.model_dir.mkdir()
        (self.model_dir/'config.json').write_text('{}')
        (self.model_dir/'model.safetensors').write_bytes(b'fake checkpoint')
        self.context = {'voice': read.voice_context(self.voice), 'model': read.model_context(self.model_dir),
                        'runtime': {'fake': True}}

    def test_gain_and_soft_limiter_are_independent_of_chunking(self):
        samples = np.array([0, .01, -.01, .1, -.1, .5, -.5, 1, -1], dtype=np.float32)
        output, limited = read.amplified_audio(samples, 12)
        np.testing.assert_allclose(output[:5], samples[:5] * 10**(12/20), atol=1e-7)
        self.assertEqual(limited, 4)
        self.assertLessEqual(np.max(np.abs(output)), .98)
        pieces = np.concatenate([read.amplified_audio(part, 12)[0] for part in np.array_split(samples, 3)])
        np.testing.assert_array_equal(pieces, output)
        unamplified, _ = read.amplified_audio(samples[:5], 0)
        np.testing.assert_array_equal(unamplified, samples[:5])

    def test_nonfinite_audio_is_rejected(self):
        for sample in (np.nan, np.inf, -np.inf):
            with self.subTest(sample=sample), self.assertRaisesRegex(ValueError, 'non-finite'):
                read.amplified_audio([sample], 12)

    def test_settings_validation(self):
        for change in ({'temperature': -1}, {'temperature': np.nan}, {'top_p': 0}, {'top_p': 1.1},
                       {'top_k': -1}, {'streaming_interval': 0}, {'max_tokens': 0},
                       {'repetition_penalty': 0}, {'gain_db': 25}, {'gain_db': np.inf}):
            with self.subTest(change=change), self.assertRaises(ValueError):
                replace(read.Settings(), **change).validate()

    def test_chunks_queue_before_generation_ends_without_draining_shared_player(self):
        player = Mock()
        model = Mock(sample_rate=24000)

        def generate(**kwargs):
            self.assertEqual(kwargs['seed'], 42)
            self.assertTrue(kwargs['stream'])
            self.assertEqual(kwargs['temperature'], .9)
            yield types.SimpleNamespace(audio=np.full(120, .1), sample_rate=24000)
            player.queue_audio.assert_called_once()
            yield types.SimpleNamespace(audio=np.full(120, -.1), sample_rate=24000)

        model.generate = generate
        with redirect_stderr(io.StringIO()):
            result = read.read_utterance(model, player, 'Hello.', read.Settings(), 42, self.root/'reads', self.context)
        player.stop.assert_not_called()
        player.flush.assert_not_called()
        self.assertEqual(result['playback'], 'queued')
        self.assertEqual(result['request']['seed'], 42)
        self.assertEqual(result['audio']['chunks'], 2)
        self.assertAlmostEqual(result['audio']['seconds'], .01)
        self.assertEqual(result['audio']['sha256'], read.hash_file(result['audio']['path']))
        saved = json.loads(Path(result['receipt']).read_text())
        self.assertEqual(saved['status'], 'complete')
        audio, rate = sf.read(result['audio']['path'])
        self.assertEqual(rate, 24000)
        self.assertAlmostEqual(audio[0], .1 * 10**(12/20), places=4)

    def test_failed_generation_preserves_queued_speech_and_leaves_error_receipt(self):
        player = Mock()
        model = Mock(sample_rate=24000)

        def generate(**kwargs):
            yield types.SimpleNamespace(audio=np.ones(120), sample_rate=24000)
            raise RuntimeError('Model failed')

        model.generate = generate
        with redirect_stderr(io.StringIO()), self.assertRaisesRegex(RuntimeError, 'Model failed'):
            read.read_utterance(model, player, 'Hello.', read.Settings(), 42, self.root/'reads', self.context)
        player.flush.assert_not_called()
        saved = json.loads(next((self.root/'reads').glob('*.json')).read_text())
        self.assertEqual(saved['status'], 'error')
        self.assertEqual(saved['request']['seed'], 42)

    def run_cli(self, args, stdin='', fail_text=None, player=None, before_generate=None):
        model = Mock(sample_rate=24000)

        def generate(**kwargs):
            if before_generate:
                before_generate(kwargs)
            if kwargs['text'] == fail_text:
                raise RuntimeError('Bad utterance')
            return iter([types.SimpleNamespace(audio=np.full(240, .1), sample_rate=24000)])

        model.generate.side_effect = generate
        load = Mock(return_value=model)
        mlx = types.ModuleType('mlx')
        mlx.core = types.ModuleType('mlx.core')
        tts = types.ModuleType('mlx_audio.tts')
        tts.load = load
        stdout, stderr = io.StringIO(), io.StringIO()
        modules = {'mlx': mlx, 'mlx.core': mlx.core, 'mlx_audio': types.ModuleType('mlx_audio'), 'mlx_audio.tts': tts}
        with patch.dict(sys.modules, modules), patch.object(read, 'runtime_context', return_value={'fake': True}), \
                patch.object(read, 'Playback', return_value=player), \
                patch('sys.stdin', io.StringIO(stdin)), redirect_stdout(stdout), redirect_stderr(stderr):
            playback_args = [] if player is not None else ['--no-play']
            code = read.main(['--model', str(self.model_dir), '--voice-dir', str(self.voice), *playback_args, *args])
        return code, [json.loads(line) for line in stdout.getvalue().splitlines()], model, load

    def test_stdin_loads_once_skips_blanks_and_reseeds_every_read(self):
        code, results, model, load = self.run_cli(
            ['--stdin', '--seed', '42', '--output-dir', str(self.root/'reads')], 'First.\n\nSecond.\nFirst.\n')
        self.assertEqual(code, 0)
        load.assert_called_once()
        self.assertEqual([call.kwargs['seed'] for call in model.generate.call_args_list], [42, 42, 42])
        self.assertEqual([call.kwargs['text'] for call in model.generate.call_args_list], ['First.', 'Second.', 'First.'])
        self.assertEqual(len(results), 3)

    def test_next_line_generates_while_previous_audio_is_still_playing(self):
        stream = GatedStream(gated=True)
        player = Playback(24000, stream_factory=Mock(return_value=stream))
        self.addCleanup(player.flush)
        self.addCleanup(stream.release.set)

        def before_generate(kwargs):
            if kwargs['text'] == 'Second.':
                timeout_seconds = 3
                self.assertTrue(stream.entered.wait(timeout_seconds))
                self.assertFalse(stream.finished_first.is_set())
                stream.release.set()

        code, results, _, load = self.run_cli(
            ['--stdin', '--seed', '42', '--output-dir', str(self.root/'reads')],
            'First.\nSecond.\n', player=player, before_generate=before_generate)
        self.assertEqual(code, 0)
        self.assertEqual(len(results), 2)
        self.assertTrue(all(result['playback'] == 'queued' for result in results))
        self.assertEqual([name for name, _ in stream.calls][-2:], ['stop', 'close'])
        self.assertFalse(player._thread.is_alive())
        load.assert_called_once()

    def test_random_seeds_are_saved_for_replay(self):
        with patch.object(read.secrets, 'randbits', side_effect=[123, 456]):
            _, results, _, _ = self.run_cli(['--stdin', '--output-dir', str(self.root/'reads')], 'First.\nSecond.\n')
        seeds = [json.loads(Path(result['receipt']).read_text())['request']['seed'] for result in results]
        self.assertEqual(seeds, [123, 456])

    def test_stdin_recovers_after_a_failed_line_without_reloading(self):
        code, results, model, load = self.run_cli(
            ['--stdin', '--seed', '42', '--output-dir', str(self.root/'reads')],
            'First.\nBad.\nLast.\n', fail_text='Bad.')
        self.assertEqual(code, 1)
        self.assertEqual(len(results), 2)
        self.assertEqual(model.generate.call_count, 3)
        load.assert_called_once()
        records = [json.loads(path.read_text()) for path in (self.root/'reads').glob('*.json')]
        self.assertEqual(sum(record['status'] == 'error' for record in records), 1)

    def test_replay_restores_settings_and_audio(self):
        _, original, _, _ = self.run_cli(['Hello.', '--seed', '987', '--temperature', '.5', '--gain-db', '6',
                                        '--instruction', 'Speak calmly.', '--output-dir', str(self.root/'original')])
        _, replayed, model, _ = self.run_cli(['--replay', original[0]['receipt'], '--output-dir', str(self.root/'replay')])
        kwargs = model.generate.call_args.kwargs
        self.assertEqual((kwargs['seed'], kwargs['temperature'], kwargs['cfg_scale']), (987, .5, 4))
        self.assertEqual(original[0]['audio']['sha256'], replayed[0]['audio']['sha256'])

    def test_replay_rejects_changed_reference(self):
        _, original, _, _ = self.run_cli(['Hello.', '--seed', '42', '--output-dir', str(self.root/'reads')])
        (self.voice/'voice.json').write_text(json.dumps({'transcript': 'Changed words.'}))
        with self.assertRaisesRegex(ValueError, 'Reference audio or transcript differs'):
            self.run_cli(['--replay', original[0]['receipt']])

    def test_model_identity_survives_directory_move_but_detects_changed_weights(self):
        original = read.model_context(self.model_dir)['identity']
        moved = self.root/'moved'
        self.model_dir.rename(moved)
        self.assertEqual(original, read.model_context(moved)['identity'])
        (moved/'model.safetensors').write_bytes(b'different weights')
        self.assertNotEqual(original, read.model_context(moved)['identity'])

    def test_stale_download_metadata_does_not_hide_weight_changes(self):
        weights = self.model_dir/'model.safetensors'
        original_hash = read.hash_file(weights)
        info = self.model_dir/'.cache/huggingface/download/model.safetensors.metadata'
        info.parent.mkdir(parents=True)
        info.write_text('revision\n' + original_hash + '\n1234\n')
        os.utime(weights, (1233, 1233))
        self.assertEqual(read.model_context(self.model_dir)['identity']['weights'][0]['sha256'], original_hash)
        weights.write_bytes(b'changed checkpoint')
        os.utime(weights, (1235, 1235))
        self.assertEqual(read.model_context(self.model_dir)['identity']['weights'][0]['sha256'], read.hash_file(weights))

    def test_cli_rejects_ambiguous_modes_invalid_seed_and_replay_overrides(self):
        for args in ([], ['Hello', '--stdin'], ['--seed', '-1', 'Hello'], ['--replay', 'read.json', '--gain-db', '12']):
            with self.subTest(args=args), redirect_stderr(io.StringIO()), self.assertRaises(SystemExit):
                read.parse_args(args)


if __name__ == '__main__':
    unittest.main()
