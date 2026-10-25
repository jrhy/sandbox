"""Download the Mac checkpoints into the ignored project-local model directory."""
import os
from pathlib import Path

root = Path(__file__).resolve().parent.parent
cache = root / '.models' / 'huggingface'
os.environ['HF_HOME'] = str(cache)
os.environ['HF_HUB_CACHE'] = str(cache / 'hub')
os.environ['HF_XET_CACHE'] = str(cache / 'xet')


def main():
    from huggingface_hub import snapshot_download

    models = [
        ('mlx-community/Breeze-TTS-2-mlx-8bit', 'breeze-tts-2-8bit',
         'c6e4a2ff6ab9afba68b7853de802273ffe23fb49'),
        ('mlx-community/parakeet-tdt-0.6b-v3', 'parakeet-tdt-0.6b-v3',
         'ed2b7e8c15f9aaa0b5772e2efb986255eaef7e15'),
        ('mlx-community/diar_sortformer_4spk-v1-fp16', 'sortformer-4spk-v1',
         '5233f9c657bf5e5535b7b79de040dab3ff9b905f'),
    ]
    for repo, folder, revision in models:
        print(f'Downloading {repo}', flush=True)
        snapshot_download(repo, revision=revision, local_dir=root / '.models' / folder)
        print(f'Ready: {folder}', flush=True)


if __name__ == '__main__':
    main()
