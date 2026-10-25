"""CPU integration checks; no model weights or network access required."""
import io,json,os,tempfile,time,unittest,wave,zipfile
from pathlib import Path
from unittest.mock import patch
os.environ['VOICE_DATA']=tempfile.mkdtemp(prefix='breeze-test-')
import server
from fastapi.testclient import TestClient
class CompanionTests(unittest.TestCase):
 def setUp(self):
  self.client=TestClient(server.app);self.headers={'Authorization':'Bearer '+server.TOKEN}
 def wait(self,j):
  for _ in range(100):
   r=self.client.get('/api/jobs/'+j,headers=self.headers).json()
   if r['status']!='running':return r
   time.sleep(.03)
  self.fail('Job timeout')
 def test_auth_and_url_validation(self):
  self.assertEqual(self.client.get('/api/health').status_code,401)
  self.assertEqual(self.client.get('/api/health',headers=self.headers).status_code,200)
  self.assertEqual(self.client.post('/api/import',headers=self.headers,json={'url':'https://127.0.0.1/private'}).status_code,400)
  self.assertEqual(self.client.get('/api/audio/not-an-id/source',headers=self.headers).status_code,400)
 def test_upload_trim_fallback_and_export(self):
  audio=io.BytesIO()
  with wave.open(audio,'wb') as w:w.setparams((1,2,24000,0,'NONE','not compressed'));w.writeframes(b'\0\0'*24000*3)
  j=self.client.post('/api/upload',headers=self.headers,content=audio.getvalue()).json()['job']
  src=self.wait(j)['result'];self.assertEqual(src['duration'],3)
  body={'source':src['id'],'start':.5,'end':2,'authorized':False}
  self.assertEqual(self.client.post('/api/clip',headers=self.headers,json=body).status_code,400)
  body['authorized']=True
  realrun=server.run
  def fail_asr(args,*a,**kw):
   if str(args[1]).endswith('transcribe.py'):raise RuntimeError('ASR unavailable in CPU test')
   return realrun(args,*a,**kw)
  with patch.object(server,'run',side_effect=fail_asr):
   j=self.client.post('/api/clip',headers=self.headers,json=body).json()['job'];result=self.wait(j)
  self.assertEqual(result['status'],'complete');clip=result['result'];self.assertIn('manually',clip['warning'])
  r=self.client.get('/api/audio/'+clip['id']+'/reference',headers=self.headers)
  with wave.open(io.BytesIO(r.content)) as w:self.assertAlmostEqual(w.getnframes()/w.getframerate(),1.5,places=2)
  r=self.client.post('/api/export',headers=self.headers,json={'clip':clip['id'],'transcript':'Corrected words.','name':'Test voice'})
  self.assertEqual(r.status_code,200)
  with zipfile.ZipFile(io.BytesIO(r.content)) as z:
   self.assertEqual(set(z.namelist()),{'reference.wav','voice.json','speak.py','README.txt'})
   self.assertEqual(json.loads(z.read('voice.json'))['transcript'],'Corrected words.')
  self.assertEqual(self.client.post('/api/synthesize',headers=self.headers,json={'clip':clip['id'],'transcript':'words','text':'Hello'}).status_code,503)
 def test_cli_passes_text_as_one_literal_argument(self):
  import subprocess,sys
  with tempfile.TemporaryDirectory() as d:
   p=Path(d);(p/'voice.json').write_text(json.dumps({'transcript':'Exact words'}));(p/'reference.wav').touch()
   import shutil
   shutil.copy(server.ROOT/'speak.py',p/'speak.py')
   repo=p/'repo';repo.mkdir();(repo/'infer.py').write_text('import sys,json; print(json.dumps(sys.argv[1:]))')
   env=dict(os.environ,BREEZE_REPO=str(repo),BREEZE_MODEL=str(p/'model'))
   text='Hello; $(not-a-command) `literal`'
   r=subprocess.run([sys.executable,str(p/'speak.py'),text],env=env,capture_output=True,text=True,check=True)
   args=json.loads(r.stdout);self.assertEqual(args[args.index('--text')+1],text)
if __name__=='__main__':unittest.main()
