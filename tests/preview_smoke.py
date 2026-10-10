#!/usr/bin/env python3
"""Exercise a running preview through its authenticated UI and model gateway."""
import argparse,json,time,urllib.request,urllib.error
from pathlib import Path
p=argparse.ArgumentParser();p.add_argument('bundle',type=Path);a=p.parse_args();root=a.bundle
session=json.loads((root/'state/browser-session.json').read_text());base=session['base_url'];token=session['token']
def request(path,body=None,base=base,key=token):
 req=urllib.request.Request(base+path,data=None if body is None else json.dumps(body).encode(),headers={'Authorization':'Bearer '+key,'Content-Type':'application/json'})
 with urllib.request.urlopen(req,timeout=180)as r:return r.read()
def api(path,body=None):return json.loads(request('/api/'+path,body))
print('Version:',api('status')['version'],flush=True)
for theme in ('light','dark','system'):
 assert api('preferences',{'theme':theme})['theme']==theme
for purpose in ('chat','coding'):
 rec=api('recommend?purpose='+purpose);assert rec['context']>=2048;print('Recommendation:',purpose,rec['model'],rec['context'],flush=True)
inv=api('inventory?context=8192&backend=auto');models=[m for m in inv['models']if m['eligible']];assert len(models)>=2
api('start',{'model':models[0]['name'],'enabled':[m['id']for m in models],'context':8192,'backend':'auto','temperature':.2,'threads':0,'port':8876})
for _ in range(90):
 status=api('status')
 if status['engine']['ready']:break
 time.sleep(1)
else:raise RuntimeError('Engine failed to become ready: '+str(status))
assert status['gateway']['stage']=='available' and not status['gateway']['model'],'Engine should not preload weights'
provider=json.loads((root/'state/provider.json').read_text());key=(root/'state/provider-key').read_text().strip()
assert len(api('provider-models')['data'])==len(models)
for m in models[:2]:
 started=time.monotonic()
 output=request('/api/chat',{'model':m['id'],'messages':[{'role':'user','content':'Say hello in one short sentence.'}]})
 assert b'data:'in output and b'[DONE]'in output,output[:500]
 status=api('status');assert status['gateway']['model']==m['name'],status
 print('Streamed from:',m['name'],'seconds:',round(time.monotonic()-started,2),flush=True)
try:request('/v1/chat/completions',{'model':'missing','messages':[]},base=provider['base_url'],key=key)
except urllib.error.HTTPError as e:assert e.code==400
else:raise AssertionError('Unknown model was accepted')
print('PASS: themes, recommendations, deferred loading, two-model routing, streaming, invalid model rejection',flush=True)
