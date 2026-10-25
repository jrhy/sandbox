"""Deterministic overlap/assembly checks and real PCM API integration tests."""
import array
import io
import json
import tempfile
import time
import unittest
import wave
from pathlib import Path
from unittest.mock import patch

from fastapi.testclient import TestClient

import server
from speakers import assemble_reference, clean_segments


def segment(speaker, start, end):
    return {'speaker': speaker, 'start': start, 'end': end}


def wav_bytes(samples, sample_rate=24000):
    output = io.BytesIO()
    with wave.open(output, 'wb') as writer:
        writer.setparams((1, 2, sample_rate, 0, 'NONE', 'not compressed'))
        writer.writeframes(array.array('h', samples).tobytes())
    return output.getvalue()


class SegmentTests(unittest.TestCase):
    def test_exclusive_intervals(self):
        cases = [
            ('overlap', [segment(0, 0, 4), segment(1, 3, 8), segment(0, 6, 10)],
             [segment(0, 0, 3), segment(1, 4, 6), segment(0, 8, 10)]),
            ('same speaker overlap', [segment(0, 0, 5), segment(0, 4, 10)],
             [segment(0, 0, 10)]),
            ('stream boundary', [segment(0, 0, 5), segment(0, 5, 10)],
             [segment(0, 0, 10)]),
            ('rounded stream boundary', [segment(0, 0, 4.999999999999), segment(0, 5, 10)],
             [segment(0, 0, 10)]),
            ('fully overlapping', [segment(0, 0, 5), segment(1, 0, 5)], []),
            ('silence', [], []),
            ('clamp', [segment(0, -2, 12)], [segment(0, 0, 10)]),
            ('short fragment', [segment(0, 1, 1.5)], []),
        ]
        for name, raw, expected in cases:
            with self.subTest(name=name):
                self.assertEqual(clean_segments(raw, 10, guard_seconds=0), expected)

    def test_guard_removes_both_edges(self):
        self.assertEqual(clean_segments([segment(0, 0, 3)], 3), [segment(0, .12, 2.88)])

    def test_assembly_preserves_only_selected_audio_and_caps_length(self):
        with tempfile.TemporaryDirectory() as directory:
            root = Path(directory)
            (root/'source.wav').write_bytes(wav_bytes([1000]*48000 + [-1000]*48000 + [2000]*48000))
            used, duration = assemble_reference(root/'source.wav', root/'reference.wav',
                                               [segment(0, 4, 6), segment(0, 0, 2)], 3.5)
            self.assertEqual(duration, 3.5)
            self.assertEqual([s['start'] for s in used], [0, 4])
            with wave.open(str(root/'reference.wav')) as reader:
                samples = array.array('h', reader.readframes(reader.getnframes()))
            self.assertEqual(set(samples), {0, 1000, 2000})
            self.assertEqual(samples.count(0), round(.12 * 24000))


class SpeakerAPITests(unittest.TestCase):
    def setUp(self):
        self.directory = tempfile.TemporaryDirectory()
        self.addCleanup(self.directory.cleanup)
        self.data_patch = patch.object(server, 'DATA', Path(self.directory.name))
        self.data_patch.start()
        self.addCleanup(self.data_patch.stop)
        self.client = TestClient(server.app)
        self.headers = {'Authorization': 'Bearer ' + server.TOKEN}

    def wait(self, response):
        self.assertEqual(response.status_code, 200, response.text)
        job = response.json()['job']
        timeout_seconds = 5
        deadline = time.monotonic() + timeout_seconds
        while time.monotonic() < deadline:
            result = self.client.get('/api/jobs/' + job, headers=self.headers).json()
            if result['status'] != 'running':
                self.assertEqual(result['status'], 'complete', result)
                return result['result']
            time.sleep(.01)
        self.fail('Speaker API job did not finish')

    def test_diarization_selection_transcription_and_export(self):
        ident, path = server.newfolder()
        (path/'source.wav').write_bytes(wav_bytes([1000]*72000 + [-1000]*48000 + [2000]*72000))
        (path/'source.json').write_text(json.dumps({'origin':'Test recording', 'duration':8}))
        model = Path(self.directory.name)/'model'
        model.mkdir()
        (model/'model.safetensors').touch()
        real_run = server.run

        def model_run(args, *a, **kw):
            if str(args[1]).endswith('diarize.py'):
                return json.dumps({'segments':[segment(0, 0, 3), segment(1, 3, 5), segment(0, 5, 8)]})
            if str(args[1]).endswith('transcribe.py'):
                return json.dumps({'text':'The assembled dialogue.'})
            return real_run(args, *a, **kw)

        self.assertEqual(self.client.post('/api/speakers', json={'source':ident}).status_code, 401)
        with patch.dict('os.environ', {'SORTFORMER_MODEL':str(model)}), patch.object(server, 'run', side_effect=model_run):
            result = self.wait(self.client.post('/api/speakers', headers=self.headers, json={'source':ident}))
            self.assertEqual(len(result['segments']), 3)
            body = {'source':ident, 'speaker':0, 'segments':[0, 2], 'max_seconds':6, 'authorized':False}
            self.assertEqual(self.client.post('/api/speaker-reference', headers=self.headers, json=body).status_code, 400)
            body['authorized'] = True
            for invalid in [[1], [-1], [1000]]:
                with self.subTest(indices=invalid):
                    self.assertEqual(self.client.post('/api/speaker-reference', headers=self.headers,
                                                     json={**body, 'segments':invalid}).status_code, 400)
            clip = self.wait(self.client.post('/api/speaker-reference', headers=self.headers, json=body))
        self.assertEqual(clip['transcript'], 'The assembled dialogue.')
        self.assertEqual(clip['speaker'], 0)
        self.assertEqual(len(clip['segments']), 2)
        response = self.client.get('/api/audio/'+clip['id']+'/reference', headers=self.headers)
        with wave.open(io.BytesIO(response.content)) as reader:
            samples = array.array('h', reader.readframes(reader.getnframes()))
        self.assertEqual(set(samples), {0, 1000, 2000})
        self.assertEqual(self.client.post('/api/export', headers=self.headers,
                                        json={'clip':clip['id'], 'transcript':clip['transcript']}).status_code, 200)
        self.assertEqual(self.client.get('/api/sources', headers=self.headers).json()[0]['id'], ident)


if __name__ == '__main__':
    unittest.main()
