'use strict';
const $=id=>document.getElementById(id);
let base='',token='',connected=false,busy=false,source=null,clip=null,previewing=false,previewEnd=0,diarization=null,savedSources=[];
const urls={};
for(let i=0;i<65;i++){const b=document.createElement('i');b.style.height=(10+Math.pow(Math.sin(i*1.71),2)*(135*Math.exp(-Math.pow((i-32)/24,2))))+'px';$('artbars').append(b)}
function message(text,error=false){$('status').textContent=text;$('status').classList.toggle('error',error)}
function ready(){$('analyzeSpeakers').disabled=!source||!connected||busy;$('restoreSource').disabled=!connected||busy||!$('savedSource').value;$('savedSource').disabled=busy;$('assembleSpeaker').disabled=!diarization||!connected||!$('permission').checked||busy||!document.querySelector('#speakerSegments input:checked');document.querySelectorAll('#speakerResults input,#speakerResults select,#speakerSegments button').forEach(el=>el.disabled=busy);['import','file'].forEach(id=>$(id).disabled=!connected||busy);['previewClip','markStart','markEnd'].forEach(id=>$(id).disabled=!source||busy);$('transcribe').disabled=!source||!connected||!$('permission').checked||busy;['generate','export'].forEach(id=>$(id).disabled=!clip||!connected||!$('transcript').value.trim()||busy);$('generate').disabled||=$('text').value.trim()==='';['load','start','end','permission'].forEach(id=>$(id).disabled=busy)}
async function request(path,options={}){const response=await fetch(base+path,{...options,headers:{Authorization:'Bearer '+token,...options.headers}});if(!response.ok){let detail;try{detail=(await response.json()).detail}catch{detail='Companion request failed ('+response.status+')'}throw Error(typeof detail==='string'?detail:JSON.stringify(detail))}return response}
async function json(path,body){return (await request(path,{method:'POST',headers:{'Content-Type':'application/json'},body:JSON.stringify(body)})).json()}
async function task(path,body,raw=false){const created=raw?await(await request(path,{method:'POST',headers:{'Content-Type':'application/octet-stream'},body})).json():await json(path,body);for(let i=0;i<1500;i++){await new Promise(r=>setTimeout(r,1500));const status=await(await request('/api/jobs/'+created.job)).json();if(status.status==='complete')return status.result;if(status.status==='error')throw Error(status.error)}throw Error('The job is still running. Check the companion terminal.')}
async function act(label,fn){if(busy)return;busy=true;ready();message(label);try{await fn()}catch(e){message(e.message||String(e),true)}finally{busy=false;ready()}}
function media(id,blob){if(urls[id])URL.revokeObjectURL(urls[id]);urls[id]=URL.createObjectURL(blob);$(id).src=urls[id];$(id).hidden=false}
function resetClip(){clip=null;$('transcript').value='';$('referenceAudio').pause();$('referenceAudio').hidden=true;resetOutput()}
function resetOutput(){$('resultAudio').pause();$('resultAudio').hidden=true;$('downloadAudio').hidden=true;$('output').hidden=false}
function videoID(value){let u;try{u=new URL(value)}catch{throw Error('Enter a valid YouTube URL.')}if(u.protocol!=='https:'||!['youtube.com','www.youtube.com','m.youtube.com','youtu.be'].includes(u.hostname))throw Error('Use an HTTPS YouTube video URL.');const id=u.hostname==='youtu.be'?u.pathname.slice(1):u.pathname==='/watch'?u.searchParams.get('v'):(/^\/(shorts|embed)\//.test(u.pathname)?u.pathname.split('/')[2]:'');if(!/^[\w-]{11}$/.test(id||''))throw Error('Use a link to one YouTube video.');return id}
$('load').onclick=()=>{try{const id=videoID($('youtube').value.trim());const f=document.createElement('iframe');f.src='https://www.youtube-nocookie.com/embed/'+id;f.title='YouTube reference video';f.allow='accelerometer; autoplay; encrypted-media; gyroscope; picture-in-picture';f.allowFullscreen=true;$('videoWrap').replaceChildren(f);message('Explore the video, then extract its audio to select an exact clip.')}catch(e){message(e.message,true)}};
async function setSource(result){source=result;previewing=false;diarization=null;$('speakerResults').hidden=true;resetClip();$('start').value=0;$('end').value=Math.min(10,result.duration).toFixed(1);$('end').max=$('start').max=result.duration;const blob=await(await request('/api/audio/'+result.id+'/source')).blob();media('sourceAudio',blob);$('waveLabel').hidden=false;$('waveLabel').textContent='Imported · '+result.duration.toFixed(1)+' seconds';try{const ctx=new AudioContext();try{const data=await ctx.decodeAudioData(await blob.arrayBuffer());drawWave(data);$('waveLabel').hidden=true}finally{await ctx.close()}}catch{}message('Audio imported. Select a clean sentence and create your reference clip.');ready()}
function drawWave(buffer){const c=$('wave');c.width=1000;c.height=200;const ctx=c.getContext('2d'),data=buffer.getChannelData(0);ctx.fillStyle='#7da566';for(let x=0;x<250;x++){const start=Math.floor(x*data.length/250),end=Math.floor((x+1)*data.length/250);let peak=0;for(let n=start;n<end;n+=Math.max(1,Math.floor((end-start)/100)))peak=Math.max(peak,Math.abs(data[n]));const h=Math.max(2,peak*170);ctx.fillRect(x*4,100-h/2,2,h)}}
$('import').onclick=()=>act('Extracting YouTube audio on the companion…',async()=>{videoID($('youtube').value.trim());await setSource(await task('/api/import',{url:$('youtube').value.trim()}))});
$('file').onchange=()=>{const file=$('file').files[0];if(!file)return;if(!connected){message('Connect the companion before uploading audio.',true);$('setup').showModal();$('file').value='';return}if(file.size>100*1024*1024){message('Choose an audio file smaller than 100 MB.',true);return}act('Uploading and decoding audio…',async()=>{await setSource(await task('/api/upload',file,true));$('file').value=''})};
function range(){const start=Number($('start').value),end=Number($('end').value);if(!source||!Number.isFinite(start)||!Number.isFinite(end)||start<0||end>source.duration+.01||end-start<1||end-start>30)throw Error('Select 1–30 seconds within the imported audio.');return {start,end}}
$('markStart').onclick=()=>{$('start').value=$('sourceAudio').currentTime.toFixed(1)};$('markEnd').onclick=()=>{$('end').value=$('sourceAudio').currentTime.toFixed(1)};
$('previewClip').onclick=()=>{try{const r=range();$('sourceAudio').currentTime=r.start;previewing=true;previewEnd=r.end;$('sourceAudio').play().catch(e=>message(e.message,true))}catch(e){message(e.message,true)}};
$('sourceAudio').ontimeupdate=()=>{if(previewing&&$('sourceAudio').currentTime>=previewEnd){$('sourceAudio').pause();previewing=false}};
$('transcribe').onclick=()=>act('Cutting your reference and loading Parakeet. The first run downloads the model…',async()=>{const r=range();clip=await task('/api/clip',{source:source.id,...r,authorized:$('permission').checked});$('transcript').value=clip.transcript;media('referenceAudio',await(await request('/api/audio/'+clip.id+'/reference')).blob());resetOutput();message(clip.warning||'Reference ready. Listen to the cut and correct the transcript, then generate speech.',!!clip.warning)} );
$('generate').onclick=()=>act('Breeze is generating speech. The first synthesis includes model loading…',async()=>{resetOutput();const result=await task('/api/synthesize',{clip:clip.id,transcript:$('transcript').value.trim(),text:$('text').value.trim(),instruction:$('instruction').value.trim()});media('resultAudio',await(await request('/api/audio/'+result.id+'/speech')).blob());$('downloadAudio').href=urls.resultAudio;$('downloadAudio').download='speech.wav';$('downloadAudio').hidden=false;$('output').hidden=true;message('Speech generated. Press play to audition your reference voice.')} );
$('export').onclick=()=>act('Packing the reference audio, corrected transcript and CLI runner…',async()=>{const response=await request('/api/export',{method:'POST',headers:{'Content-Type':'application/json'},body:JSON.stringify({clip:clip.id,transcript:$('transcript').value.trim(),name:$('voiceName').value.trim()||'My voice'})});const url=URL.createObjectURL(await response.blob());const a=document.createElement('a');a.href=url;a.download='breeze-voice.zip';a.click();setTimeout(()=>URL.revokeObjectURL(url),10000);message('Voice pack exported. Unzip it and use speak.py with your Breeze installation.')} );
$('permission').onchange=ready;$('transcript').oninput=()=>{resetOutput();ready()};$('text').oninput=()=>{$('chars').textContent=$('text').value.length.toLocaleString()+' / 3,000';ready()};$('text').oninput();
document.querySelectorAll('[data-direction]').forEach(b=>b.onclick=()=>{$('instruction').value=b.dataset.direction});
$('connection').onclick=$('setupLink').onclick=()=>$('setup').showModal();
function storedValue(key){try{return localStorage.getItem(key)||''}catch{return ''}}
function saveValue(key,value){try{localStorage.setItem(key,value)}catch{}}
function tokenKey(endpoint){return 'breeze-companion-token:'+new URL(endpoint).href.replace(/\/$/,'')}
function restoreToken(){try{$('token').value=storedValue(tokenKey($('endpoint').value.trim()))}catch{$('token').value=''}}
$('endpoint').value=storedValue('breeze-companion')||(location.port==='8765'?location.origin:'http://127.0.0.1:8765');
restoreToken();
$('endpoint').onchange=restoreToken;
async function connectCompanion(automatic=false){
  if(busy){$('setupStatus').textContent='Wait for the current operation before changing companions.';return}
  try{
    const u=new URL($('endpoint').value.trim());
    if(u.protocol!=='https:'&&!(u.protocol==='http:'&&['localhost','127.0.0.1','[::1]'].includes(u.hostname)))throw Error('Use HTTPS for remote companions, or HTTP on localhost.');
    if(u.username||u.password||u.search||u.hash)throw Error('Use a plain companion address, without credentials or query parameters.');
    const nextBase=u.href.replace(/\/$/,'');
    const nextToken=$('token').value.trim();
    if(!nextToken)throw Error('Enter the companion token.');
    $('connect').disabled=true;$('setupStatus').textContent='Checking companion configuration…';
    const connectionTimeoutMilliseconds=15000;
    const response=await fetch(nextBase+'/api/health',{headers:{Authorization:'Bearer '+nextToken},signal:AbortSignal.timeout(connectionTimeoutMilliseconds)});
    if(!response.ok){
      if(response.status===401||response.status===403){
        try{localStorage.removeItem(tokenKey(nextBase))}catch{}
        $('token').value='';
      }
      throw Error('Connection rejected. Check the token and companion address.');
    }
    const result=await response.json();
    if(base!==nextBase||token!==nextToken){source=null;diarization=null;$('speakerResults').hidden=true;resetClip();$('sourceAudio').hidden=true;$('sourceAudio').pause()}
    base=nextBase;token=nextToken;connected=true;
    saveValue('breeze-companion',base);saveValue(tokenKey(base),token);
    $('connection').classList.add('connected');$('connectionLabel').textContent='Companion connected';$('setup').close();
    await loadSavedSources();
    message(result.breeze_configured?(result.ffmpeg&&result.yt_dlp?'Companion connected. Runtime paths configured; model loading is checked on first use.':'Connected. Install ffmpeg and yt-dlp on the companion for audio import.'):'Connected. Set BREEZE_REPO and BREEZE_MODEL on the companion before synthesis.');
  }catch(e){
    $('setupStatus').textContent=e.message+' If local access is blocked, open the local studio on the companion host.';
    if(automatic)message('Could not restore the companion connection. Open Connect companion to retry or enter a new token.',true);
  }finally{$('connect').disabled=false;ready()}
}
$('connect').onclick=()=>connectCompanion();
ready();

async function loadSavedSources(){
  savedSources=await(await request('/api/sources')).json();
  const select=$('savedSource');select.replaceChildren(new Option('Select a previous recording…',''));
  for(const item of savedSources)select.add(new Option(item.origin+' · '+item.duration.toFixed(1)+'s · '+item.id.slice(0,8),item.id));
}
$('savedSource').onchange=ready;
$('restoreSource').onclick=()=>act('Opening recording…',async()=>{const item=savedSources.find(s=>s.id===$('savedSource').value);if(item)await setSource(item)});
$('analyzeSpeakers').onclick=()=>act('Sortformer is finding speakers and excluding overlapping speech…',async()=>{
  const result=await task('/api/speakers',{source:source.id});diarization=result;
  const speakers=[...new Set(result.segments.map(s=>s.speaker))].sort();
  $('speakerSelect').replaceChildren(...speakers.map(s=>new Option('Speaker '+(s+1),s)));
  $('speakerResults').hidden=!speakers.length;renderSpeaker();
  message(speakers.length?'Found '+speakers.length+' speakers with usable speech. Preview excerpts, uncheck mistakes, then confirm permission and assemble a reference.':'No clean speaker excerpts were found. Try another recording or make a manual cut.');
});
function renderSpeaker(){
  const selected=Number($('speakerSelect').value),container=$('speakerSegments');container.replaceChildren();
  if(!diarization)return;
  let seconds=0,count=0;
  diarization.segments.forEach((segment,index)=>{
    if(segment.speaker!==selected)return;
    seconds+=segment.end-segment.start;count++;
    const row=document.createElement('div');row.className='speaker-segment';
    const label=document.createElement('label'),check=document.createElement('input');
    check.type='checkbox';check.value=index;check.checked=true;check.onchange=ready;
    label.append(check,document.createTextNode(segment.start.toFixed(2)+'–'+segment.end.toFixed(2)+'s'));
    const preview=document.createElement('button');preview.className='outline';preview.textContent='Preview';
    preview.onclick=()=>{previewing=true;previewEnd=segment.end;$('sourceAudio').currentTime=segment.start;$('sourceAudio').play().catch(e=>message(e.message,true))};
    row.append(label,preview);container.append(row);
  });
  $('speakerSummary').textContent=count+' clean excerpts · '+seconds.toFixed(1)+' seconds available. Review for mistaken assignments.';ready();
}
$('speakerSelect').onchange=renderSpeaker;
$('assembleSpeaker').onclick=()=>act('Assembling selected dialogue and transcribing the complete reference…',async()=>{
  const maxSeconds=Number($('speakerDuration').value);
  if(!Number.isFinite(maxSeconds)||maxSeconds<1||maxSeconds>30)throw Error('Choose a reference length from 1 to 30 seconds.');
  const segments=[...document.querySelectorAll('#speakerSegments input:checked')].map(el=>Number(el.value));
  clip=await task('/api/speaker-reference',{source:source.id,speaker:Number($('speakerSelect').value),segments,max_seconds:maxSeconds,authorized:$('permission').checked});
  $('transcript').value=clip.transcript;media('referenceAudio',await(await request('/api/audio/'+clip.id+'/reference')).blob());
  resetOutput();message(clip.warning||'Speaker reference ready: '+clip.duration.toFixed(1)+' seconds from '+clip.segments.length+' excerpts. Listen and correct the transcript before generating.',!!clip.warning);
});

if($('token').value)connectCompanion(true);
