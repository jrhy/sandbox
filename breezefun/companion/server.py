"""Private local companion for Breeze Voice Studio. See README.md."""
import io, json, os, secrets, shutil, subprocess, sys, threading, uuid, wave, zipfile
from pathlib import Path
from urllib.parse import urlparse, parse_qs
from fastapi import FastAPI, Request, HTTPException, Depends
from fastapi.responses import FileResponse, Response
from fastapi.staticfiles import StaticFiles
from fastapi.middleware.cors import CORSMiddleware
from pydantic import BaseModel, Field
from speakers import clean_segments, assemble_reference

ROOT = Path(__file__).resolve().parent
DATA = Path(os.environ.get('VOICE_DATA', str(ROOT / 'data'))).resolve()
DATA.mkdir(parents=True, exist_ok=True)
TOKEN = os.environ.get('VOICE_TOKEN') or secrets.token_urlsafe(32)
REPO = Path(os.environ.get('BREEZE_REPO', str(ROOT / 'breeze-tts'))).resolve()
MODEL = Path(os.environ.get('BREEZE_MODEL', str(ROOT / 'breeze-tts-2'))).resolve()
ASR_PYTHON = os.environ.get('PARAKEET_PYTHON', sys.executable)
BREEZE_PYTHON = os.environ.get('BREEZE_PYTHON', sys.executable)
app = FastAPI()
app.add_middleware(CORSMiddleware, allow_origins=[os.environ.get('STUDIO_ORIGIN', 'https://breeze-voice-studio.johndoe736.chatgpt.site'), 'http://127.0.0.1:8765', 'http://localhost:8765'], allow_methods=['GET','POST'], allow_headers=['Authorization','Content-Type'])

def auth(request: Request):
    if not secrets.compare_digest(request.headers.get('authorization',''), 'Bearer ' + TOKEN):
        raise HTTPException(401, 'Use the companion token printed at startup.')

def folder(ident):
    try: ident = str(uuid.UUID(ident))
    except ValueError: raise HTTPException(400, 'Invalid identifier')
    p = DATA / ident
    if not p.is_dir(): raise HTTPException(404, 'Source or clip not found')
    return p

def run(args, timeout=600):
    result = subprocess.run([str(a) for a in args], capture_output=True, text=True, timeout=timeout)
    if result.returncode: raise RuntimeError(result.stderr[-1800:] or result.stdout[-1800:] or 'Process failed')
    return result.stdout

JOBS = {}
LOCK = threading.Lock()
def job(fn):
    ident = str(uuid.uuid4()); JOBS[ident] = {'status':'running'}
    def work():
        try:
            with LOCK: result = fn()
            JOBS[ident] = {'status':'complete','result':result}
        except Exception as e: JOBS[ident] = {'status':'error','error':str(e)}
    threading.Thread(target=work, daemon=True).start()
    return {'job': ident}

def newfolder():
    ident = str(uuid.uuid4()); p=DATA/ident; p.mkdir(); return ident,p

def normalize(src, dest):
    run(['ffmpeg','-nostdin','-y','-i',src,'-t','7200','-vn','-ac','1','-ar','24000',dest])

def source_result(ident,p,origin):
    with wave.open(str(p/'source.wav')) as w: duration=w.getnframes()/w.getframerate()
    (p/'source.json').write_text(json.dumps({'origin':origin,'duration':duration}))
    return {'id':ident,'duration':duration,'origin':origin}

def finish_reference(ident, path, data):
    run(['ffmpeg','-nostdin','-y','-i',path/'reference.wav','-ar','16000',path/'asr.wav'])
    warning = ''
    try:
        transcription_timeout_seconds = 900
        output = run([ASR_PYTHON, ROOT/'transcribe.py', path/'asr.wav'], transcription_timeout_seconds)
        text = json.loads(output.strip().splitlines()[-1])['text']
    except Exception as error:
        text = ''
        warning = 'Parakeet could not transcribe this clip. Enter its exact transcript manually. ' + str(error)
    data.update(transcript=text, authorized=True, warning=warning)
    (path/'voice.json').write_text(json.dumps(data, indent=2))
    return {'id':ident, **data}

@app.get('/api/sources', dependencies=[Depends(auth)])
def sources():
    results = []
    for path in sorted(DATA.glob('*/source.json'), key=lambda p: p.stat().st_mtime, reverse=True):
        results.append({'id':path.parent.name, **json.loads(path.read_text())})
    return results

class AnalyzeSpeakers(BaseModel):
    source: str

@app.post('/api/speakers', dependencies=[Depends(auth)])
def analyze_speakers(body: AnalyzeSpeakers):
    path = folder(body.source)
    if not (path/'source.json').is_file():
        raise HTTPException(400, 'Choose an imported recording first.')
    model = Path(os.environ.get('SORTFORMER_MODEL', ''))
    if not (model/'model.safetensors').is_file():
        raise HTTPException(503, 'Install the Sortformer checkpoint and set SORTFORMER_MODEL.')
    def work():
        cache = path/'speakers.json'
        if cache.is_file():
            return json.loads(cache.read_text())
        diarization_timeout_seconds = 1800
        output = run([ASR_PYTHON, ROOT/'diarize.py', path/'source.wav'], diarization_timeout_seconds)
        raw = json.loads(output.strip().splitlines()[-1])['segments']
        duration = json.loads((path/'source.json').read_text())['duration']
        segments = clean_segments(raw, duration)
        result = {'source':body.source, 'segments':segments, 'model':'sortformer-4spk-v1',
                  'note':'Predicted overlaps excluded; edges trimmed by 0.12 seconds. Review speaker assignments.'}
        cache.write_text(json.dumps(result, indent=2))
        return result
    return job(work)

class SpeakerReference(BaseModel):
    source: str
    speaker: int = Field(ge=0, le=3)
    segments: list[int] = Field(min_length=1, max_length=2000)
    max_seconds: float = Field(default=20, ge=1, le=30, allow_inf_nan=False)
    authorized: bool = False

@app.post('/api/speaker-reference', dependencies=[Depends(auth)])
def speaker_reference(body: SpeakerReference):
    path = folder(body.source)
    if not body.authorized:
        raise HTTPException(400, 'Confirm permission to use this voice.')
    if not (path/'speakers.json').is_file():
        raise HTTPException(400, 'Analyze this recording first.')
    candidates = json.loads((path/'speakers.json').read_text())['segments']
    selected = []
    for index in sorted(set(body.segments)):
        if index < 0 or index >= len(candidates) or candidates[index]['speaker'] != body.speaker:
            raise HTTPException(400, 'Select segments belonging to the chosen speaker.')
        selected.append(candidates[index])
    def work():
        ident, destination = newfolder()
        used, duration = assemble_reference(path/'source.wav', destination/'reference.wav', selected, body.max_seconds)
        data = {'source':json.loads((path/'source.json').read_text())['origin'],
                'source_id':body.source, 'speaker':body.speaker, 'segments':used,
                'duration':duration, 'gap_seconds':0.12}
        return finish_reference(ident, destination, data)
    return job(work)

@app.get('/api/health',dependencies=[Depends(auth)])
def health():
    return {'breeze_configured':(REPO/'infer.py').is_file() and MODEL.is_dir(), 'ffmpeg':bool(shutil.which('ffmpeg')), 'yt_dlp':bool(shutil.which('yt-dlp')), 'parakeet':'nvidia/parakeet-tdt-0.6b-v3', 'note':'Configured paths do not verify loaded models or GPU readiness.'}

@app.get('/api/jobs/{ident}',dependencies=[Depends(auth)])
def status(ident:str):
    if ident not in JOBS: raise HTTPException(404,'Job not found; the companion may have restarted.')
    return JOBS[ident]

class Import(BaseModel):
    url:str
@app.post('/api/import',dependencies=[Depends(auth)])
def import_url(body:Import):
    u=urlparse(body.url)
    if u.scheme!='https' or u.hostname not in ['youtube.com','www.youtube.com','m.youtube.com','youtu.be'] or u.port is not None: raise HTTPException(400,'Enter a standard HTTPS YouTube video URL.')
    video=u.path.strip('/') if u.hostname=='youtu.be' else (parse_qs(u.query).get('v',[''])[0] if u.path=='/watch' else u.path.split('/')[-1] if u.path.startswith(('/shorts/','/embed/')) else '')
    import re
    if not re.fullmatch(r'[A-Za-z0-9_-]{11}',video): raise HTTPException(400,'A single YouTube video URL is required.')
    url='https://www.youtube.com/watch?v='+video
    def work():
        ident,p=newfolder()
        run(['yt-dlp','--no-playlist','--no-live-from-start','--match-filter','!is_live & duration <= 7200','--max-filesize','250M','-f','bestaudio','-o',str(p/'input.%(ext)s'),url],900)
        files=list(p.glob('input.*'))
        if len(files)!=1: raise RuntimeError('No audio downloaded. Upload an authorized audio file instead.')
        normalize(files[0],p/'source.wav'); files[0].unlink()
        return source_result(ident,p,url)
    return job(work)

@app.post('/api/upload',dependencies=[Depends(auth)])
async def upload(request:Request):
    ident,p=newfolder(); src=p/'upload.bin'; size=0
    with src.open('wb') as f:
        async for chunk in request.stream():
            size+=len(chunk)
            if size>100*1024*1024:
                src.unlink(); raise HTTPException(413,'Upload limit is 100 MB.')
            f.write(chunk)
    def work():
        normalize(src,p/'source.wav');src.unlink();return source_result(ident,p,'Uploaded audio')
    return job(work)

class Clip(BaseModel):
    source:str
    start:float=Field(ge=0)
    end:float=Field(gt=0)
    authorized:bool=False
@app.post('/api/clip',dependencies=[Depends(auth)])
def clip(body:Clip):
    p=folder(body.source)
    if not body.authorized: raise HTTPException(400,'Confirm permission to use this voice.')
    duration=json.loads((p/'source.json').read_text())['duration']
    if not 1<=body.end-body.start<=30 or body.end>duration+0.01: raise HTTPException(400,'Select 1–30 seconds within the source.')
    def work():
        ident,c=newfolder()
        run(['ffmpeg','-nostdin','-y','-ss',body.start,'-i',p/'source.wav','-t',body.end-body.start,'-ac','1','-ar','24000',c/'reference.wav'])
        data={'source':json.loads((p/'source.json').read_text())['origin'],'start':body.start,'end':body.end}
        return finish_reference(ident, c, data)
    return job(work)

class Synthesis(BaseModel):
    clip:str
    transcript:str=Field(min_length=1,max_length=5000)
    text:str=Field(min_length=1,max_length=3000)
    instruction:str=Field(default='',max_length=500)
@app.post('/api/synthesize',dependencies=[Depends(auth)])
def synthesize(body:Synthesis):
    p=folder(body.clip)
    if not (p/'reference.wav').is_file(): raise HTTPException(400,'Create a reference clip first.')
    if not (REPO/'infer.py').is_file() or not MODEL.is_dir(): raise HTTPException(503,'Set BREEZE_REPO and BREEZE_MODEL on the companion host.')
    def work():
        ident,c=newfolder()
        args=[BREEZE_PYTHON,REPO/'infer.py',MODEL,'--ref-audio',p/'reference.wav','--ref-text',body.transcript,'--text',body.text,'--output',c/'speech.wav']
        if body.instruction.strip(): args+=['--instruction',body.instruction,'--cfg-scale','4']
        run(args,1200)
        return {'id':ident}
    return job(work)

@app.get('/api/audio/{ident}/{kind}',dependencies=[Depends(auth)])
def audio(ident:str,kind:str):
    if kind not in ['source','reference','speech']:raise HTTPException(400,'Invalid audio kind')
    p=folder(ident)/(kind+'.wav')
    if not p.is_file():raise HTTPException(404,'Audio not found')
    return FileResponse(p,media_type='audio/wav')

class Export(BaseModel):
    clip:str
    transcript:str=Field(min_length=1,max_length=5000)
    name:str=Field(default='My voice',max_length=100)
@app.post('/api/export',dependencies=[Depends(auth)])
def export(body:Export):
    p=folder(body.clip)
    if not (p/'voice.json').is_file():raise HTTPException(400,'Create a reference clip first.')
    data=json.loads((p/'voice.json').read_text()); data.update(name=body.name,transcript=body.transcript,format='breeze-reference-v1',model='BreezeBlue/Breeze-TTS-2')
    out=io.BytesIO()
    with zipfile.ZipFile(out,'w',zipfile.ZIP_DEFLATED) as z:
        z.write(p/'reference.wav','reference.wav');z.writestr('voice.json',json.dumps(data,indent=2));z.write(ROOT/'speak.py','speak.py')
        z.writestr('README.txt','Reference voice pack, not trained model weights. Requires the Breeze TTS 2 runtime and checkpoint. Set BREEZE_REPO and BREEZE_MODEL, then: python speak.py "Your text" --output speech.wav\nResearch/non-commercial use per Breeze model license. Keep the reference and transcript together.\n')
    return Response(out.getvalue(),media_type='application/zip',headers={'Content-Disposition':'attachment; filename="breeze-voice.zip"'})

app.mount('/',StaticFiles(directory=str(ROOT/'studio'),html=True),name='studio')
if __name__=='__main__':
    import uvicorn
    print('Local studio: http://127.0.0.1:8765\nCompanion token: '+TOKEN,flush=True)
    uvicorn.run(app,host='127.0.0.1',port=8765)
