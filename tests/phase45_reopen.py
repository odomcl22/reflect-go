#!/usr/bin/env python3
"""Check real duplicate launch, authenticated reconnect, crash restart and quit."""
import argparse,json,os,subprocess,time,urllib.request
from pathlib import Path
p=argparse.ArgumentParser();p.add_argument('--bundle',type=Path,required=True);a=p.parse_args()
bundle=a.bundle.resolve();binary=bundle/'bin/macos-arm64/reflectgo';record=bundle/'state/browser-session.json'
env={'PATH':'/usr/bin:/bin:/usr/sbin:/sbin','LANG':'en_US.UTF-8','REFLECT_GO_NO_BROWSER':'1'}
opener=urllib.request.build_opener(urllib.request.ProxyHandler({}));report=[]
def wait(fn):
    end=time.monotonic()+15
    while time.monotonic()<end:
        try:
            if value:=fn():return value
        except (OSError,ValueError):pass
        time.sleep(.1)
    raise RuntimeError('Timed out')
def api(s,path,data=None):
    req=urllib.request.Request(s['base_url']+'/api/'+path,data=None if data is None else json.dumps(data).encode(),headers={'Authorization':'Bearer '+s['token'],'Content-Type':'application/json'})
    with opener.open(req,timeout=15)as r:return json.load(r)
child=None
try:
    child=subprocess.Popen([str(binary),'ui'],env=env,stdout=subprocess.DEVNULL,stderr=subprocess.DEVNULL)
    session=wait(lambda:json.loads(record.read_text()));status=api(session,'status');assert set(status['clients'])=={'codex','claude','opencode'} and all(status['clients'].values())
    second=subprocess.run([str(binary),'ui'],env=env,text=True,capture_output=True,timeout=15)
    assert second.returncode==0 and 'Reopened' in second.stdout,(second.returncode,second.stderr)
    assert json.loads(record.read_text())==session and child.poll()is None
    report.append('second launch reconnects to the same authenticated UI; no duplicate engine')
    child.kill();child.wait(timeout=10)
    child=subprocess.Popen([str(binary),'ui'],env=env,stdout=subprocess.DEVNULL,stderr=subprocess.DEVNULL)
    fresh=wait(lambda:(s if (s:=json.loads(record.read_text()))['token']!=session['token'] else None));api(fresh,'status')
    report.append('stale record after force termination is replaced on restart')
    api(fresh,'quit',{});child.wait(timeout=10);assert child.returncode==0 and not record.exists()
    report.append('Quit exits cleanly and removes reconnect credentials')
    print(json.dumps(report,indent=2))
    out=Path(__file__).resolve().parents[1]/'docs/evidence/phase-4.5/reopen.json';out.write_text(json.dumps(report,indent=2)+'\n')
finally:
    if child and child.poll()is None:child.terminate();child.wait(timeout=15)
