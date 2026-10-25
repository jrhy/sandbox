import json
import os
import sys

if os.environ.get('PARAKEET_BACKEND') == 'mlx':
    from parakeet_mlx import from_pretrained

    model = from_pretrained(os.environ.get(
        'PARAKEET_MODEL', 'mlx-community/parakeet-tdt-0.6b-v3'))
    text = model.transcribe(sys.argv[1]).text
else:
    import nemo.collections.asr as nemo_asr

    model = nemo_asr.models.ASRModel.from_pretrained(
        model_name='nvidia/parakeet-tdt-0.6b-v3')
    result = model.transcribe([sys.argv[1]])[0]
    text = result if isinstance(result, str) else result.text

print(json.dumps({'text': text}))
