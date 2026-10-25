"""Use an exported reference voice with the official Breeze CLI."""
import argparse,json,os,subprocess,sys
from pathlib import Path
p=argparse.ArgumentParser();p.add_argument('text');p.add_argument('--output',default='speech.wav');p.add_argument('--instruction',default='');a=p.parse_args()
root=Path(__file__).resolve().parent
voice=json.loads((root/'voice.json').read_text())
repo=os.environ.get('BREEZE_REPO');model=os.environ.get('BREEZE_MODEL')
if not repo or not model:p.error('Set BREEZE_REPO and BREEZE_MODEL to the runtime and checkpoint directories.')
args=[os.environ.get('BREEZE_PYTHON',sys.executable),str(Path(repo)/'infer.py'),model,'--ref-audio',str(root/'reference.wav'),'--ref-text',voice['transcript'],'--text',a.text,'--output',str(Path(a.output).resolve())]
if a.instruction:args+=['--instruction',a.instruction,'--cfg-scale','4']
subprocess.run(args,check=True)
