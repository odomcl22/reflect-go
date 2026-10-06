#!/usr/bin/env python3
"""Real local browser API, generation, conflict and owner-crash checks on this host."""
import argparse,html,json,os,re,socket,subprocess,time,urllib.request,urllib.error
from pathlib import Path
p=argparse.ArgumentParser();p.add_argument('--bundle',type=Path,required=True);p.add_argument('--port',type=int,default=18877);a=p.parse_args();bundle=a.bundle.resolve();binary=bundle/'bin/macos-arm64/reflectgo';report=[]

def wait_for(fn,seconds=90):
 end=time.monotonic()+seconds
 while time.monotonic()<end:
  value=fn()
  if value:return value
  time.sleep(.25)
 raise RuntimeError('Timed out waiting for condition')

def launch():
 page=bundle/'state/OPEN-WORKSTATION.html'
 if page.exists():raise RuntimeError('Another browser workstation may already be open')
 log=(bundle/'state/acceptance-ui.log').open('w');child=subprocess.Popen([str(binary),'ui'],env={**os.environ,'REFLECT_GO_NO_BROWSER':'1'},stdout=log,stderr=log)
 wait_for(page.exists,20);uri=html.unescape(re.search(r'href="([^"]+)"',page.read_text())[1]);base,token=uri.rsplit('/#',1)
 def request(route,payload=None,headers=None,status=200):
  h={'Authorization':'Bearer '+token};h.update(headers or {});data=None
  if payload is not None:data=json.dumps(payload).encode();h['Content-Type']='application/json'
  req=urllib.request.Request(base+route,data=data,headers=h)
  try:r=urllib.request.urlopen(req,timeout=180)
  except urllib.error.HTTPError as e:r=e
  raw=r.read();assert r.status==status,(route,r.status,raw[:200]);return json.loads(raw) if r.headers.get_content_type()=='application/json' else raw
 return child,log,request,page

def port_free():
 with socket.socket()as s:
  s.setsockopt(socket.SOL_SOCKET,socket.SO_REUSEADDR,1)
  try:s.bind(('127.0.0.1',a.port));return True
  except OSError:return False

(bundle/'state').mkdir(exist_ok=True);child,log,api,page=launch()
try:
 api('/api/status',headers={'Authorization':''},status=401)
 api('/api/status',headers={'Origin':'https://example.invalid'},status=403)
 api('/api/status',headers={'Host':'example.invalid'},status=403)
 api('/api/start',status=405)
 inventory=api('/api/inventory?context=2048&backend=auto');assert inventory['hardware']['backend']['kind']=='metal',inventory['hardware']['backend']
 model=next(m['name'] for m in inventory['models'] if '1.5b' in m['name'])
 prefs={'model':model,'context':2048,'backend':'auto','threads':0,'temperature':.2,'port':a.port}
 api('/api/start',{**prefs,'unexpected':True},status=400)
 report.append('browser token, origin, host and mutation validation')
 # A foreign listener must survive a failed start.
 with socket.socket()as foreign:
  foreign.bind(('127.0.0.1',a.port));foreign.listen()
  api('/api/start',prefs);wait_for(lambda:not api('/api/status')['engine']['active']);assert foreign.fileno()>0
  assert 'port' in api('/api/status')['engine']['error'].lower()
 report.append('busy port fails without stopping foreign listener')
 api('/api/start',prefs);wait_for(lambda:api('/api/status')['engine']['ready'],150)
 api('/api/start',prefs,status=400)
 answer=api('/api/test',{})['response'];assert answer.strip();print('Real response:',answer[:250],flush=True)
 status=wait_for(lambda:(s if (s:=api('/api/status')['engine'])['generated']>0 and s['residentBytes']>0 else None))
 assert status['speed']>0;report.append({'generation':True,'backend':status['backend'],'tokens_per_second':status['speed'],'resident_bytes':status['residentBytes']})
 api('/api/stop',{});wait_for(lambda:not api('/api/status')['engine']['active']);wait_for(port_free)
 # Switch to the coding model at full default context; force termination must close ownership pipe.
 coding=next(m['name'] for m in inventory['models'] if '4B' in m['name'])
 api('/api/start',{**prefs,'model':coding,'context':16384});wait_for(lambda:api('/api/status')['engine']['ready'],180)
 report.append('1.5B unload then 4B/16K model switch')
 child.kill();child.wait(timeout=10);wait_for(port_free,12);report.append('force-killed UI owner releases real engine listener')
 # The prior session is dead; stale reconnect page is removable test state.
 page.unlink(missing_ok=True);log.close();child,log,api,page=launch()
 api('/api/start',{**prefs,'backend':'cpu'});wait_for(lambda:api('/api/status')['engine']['ready'],150)
 answer=api('/api/test',{})['response'];assert answer.strip();report.append('CPU generation after crash/restart')
 api('/api/quit',{});child.wait(timeout=15);assert child.returncode==0;wait_for(port_free);assert not page.exists();report.append('Quit stops provider and UI, removes reconnect token page')
 print(json.dumps(report,indent=2),flush=True)
 (bundle/'state/phase4-acceptance.json').write_text(json.dumps(report,indent=2)+'\n')
finally:
 if child.poll()is None:child.terminate();child.wait(timeout=20)
 log.close()
