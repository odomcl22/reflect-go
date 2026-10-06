#!/usr/bin/env python3
"""Exercise the shipped portable clients against a real model and isolated profiles."""
import argparse,json,os,subprocess,time,urllib.request,threading,http.server,http.client,shutil,signal
from pathlib import Path

p=argparse.ArgumentParser();p.add_argument('--bundle',type=Path,required=True);p.add_argument('--clients',nargs='+',default=['codex','claude','opencode']);p.add_argument('--codex-native-sandbox',action='store_true');p.add_argument('--workflow',action='store_true');a=p.parse_args()
root=Path(__file__).resolve().parents[1];bundle=a.bundle.resolve();binary=bundle/'bin/macos-arm64/reflectgo'
evidence=root/'docs/evidence/phase-4.5';evidence.mkdir(parents=True,exist_ok=True)
project=root/'.build/phase45/client-project';project.mkdir(parents=True,exist_ok=True)
(project/'message.txt').write_text('The portable acceptance marker is ORCHARD-742.\n')
state=bundle/'state';state.mkdir(exist_ok=True)
minimal_env={'PATH':'/usr/bin:/bin:/usr/sbin:/sbin','LANG':'en_US.UTF-8','TERM':'xterm-256color'}
# Deny external traffic and access to the user's personal files. Both writes and
# reads in the user home are limited to this test's bundle and disposable project.
quote=lambda path:json.dumps(str(path))
profile='(version 1)(allow default)(deny network-outbound)(allow network-outbound (remote ip "localhost:*"))(deny file-write*)(allow file-write* (subpath '+quote(state)+') (subpath '+quote(project)+') (subpath "/private/tmp") (subpath "/dev"))(deny file-read* (subpath '+quote(Path.home())+'))(allow file-read* (subpath '+quote(bundle)+') (subpath '+quote(project)+'))'
profile+='(allow file-read-metadata (subpath '+quote(Path.home())+'))'
sandbox=['/usr/bin/sandbox-exec','-p',profile]
canary=project.parent/'outside-project-canary.txt';canary.write_text('must stay outside the client sandbox')
probe=subprocess.run(sandbox+['/bin/cat',str(canary)],env=minimal_env,capture_output=True)
assert probe.returncode!=0 and b'must stay' not in probe.stdout,'Personal-file read guard failed'
class Capture(http.server.BaseHTTPRequestHandler):
    def log_message(self,*args):pass
    def forward(self):
        body=self.rfile.read(int(self.headers.get('Content-Length','0')))
        if body and self.path.startswith('/v1/responses'):
            n=len(list(evidence.glob('codex-request-*.json')))
            (evidence/('codex-request-'+str(n)+'.json')).write_bytes(body)
        connection=http.client.HTTPConnection('127.0.0.1',18885,timeout=180)
        connection.request(self.command,self.path,body,{k:v for k,v in self.headers.items() if k.lower() in ['authorization','x-api-key','anthropic-version','anthropic-beta','content-type']})
        response=connection.getresponse();self.send_response(response.status);self.send_header('Content-Type',response.getheader('Content-Type','application/json'));self.end_headers()
        try:
            while chunk:=response.read1(65536):self.wfile.write(chunk);self.wfile.flush()
        except (BrokenPipeError,ConnectionResetError):pass
        finally:connection.close()
    do_GET=forward
    do_POST=forward
proxy=http.server.ThreadingHTTPServer(('127.0.0.1',18886),Capture)
threading.Thread(target=proxy.serve_forever,daemon=True).start()
out=(evidence/'provider.log').open('w')
engine=subprocess.Popen([str(binary),'serve','--model','Qwen3-4B-Instruct-2507-Q4_K_M.gguf','--context','16384','--port','18885'],stdout=out,stderr=out,env=minimal_env)
opener=urllib.request.build_opener(urllib.request.ProxyHandler({}));report=[]
try:
    deadline=time.monotonic()+300
    while time.monotonic()<deadline:
        if engine.poll()is not None:raise RuntimeError('Provider exited; read provider.log')
        try:
            key=(state/'provider-key').read_text().strip();req=urllib.request.Request('http://127.0.0.1:18885/v1/models',headers={'Authorization':'Bearer '+key})
            with opener.open(req,timeout=2) as r:
                if r.status==200:break
        except Exception:time.sleep(.5)
    else:raise RuntimeError('Provider readiness timeout')
    provider_record=json.loads((state/'provider.json').read_text());provider_record['base_url']='http://127.0.0.1:18886';(state/'provider.json').write_text(json.dumps(provider_record))
    for name in a.clients:
        started=time.monotonic();path=evidence/(name+('-workflow.txt' if a.workflow else '-read.txt'))
        task='Read message.txt in the current project using your file tools, and reply with its exact acceptance marker. Do not change any files.'
        command=([] if name=='codex' and a.codex_native_sandbox else sandbox)+[str(binary),'connect',name,'--project',str(project),'--task',task]
        testenv=minimal_env
        if a.workflow:
            fixture=project/name;fixture.mkdir(exist_ok=True)
            for file in ['math.sh','test.sh']:shutil.copyfile(root/'tests/fixtures/agent-project'/file,fixture/file)
            command=[str(root/'.build/phase45/harness-tests'),'-test.run','^TestPortableHarnessWorkflow$','-test.v','-test.timeout','300s']
            testenv={**minimal_env,'REFLECT_GO_TEST_BUNDLE':str(bundle),'REFLECT_GO_TEST_CLIENT':name,'REFLECT_GO_TEST_PROJECT':str(fixture),'REFLECT_GO_TEST_PROFILE':profile}
        with path.open('w') as log:
            child=subprocess.Popen(command,stdout=log,stderr=subprocess.STDOUT,env=testenv,start_new_session=True)
            try:
                child.wait(timeout=150);r=child
            except subprocess.TimeoutExpired:
                os.killpg(child.pid,signal.SIGTERM)
                try:child.wait(timeout=5)
                except subprocess.TimeoutExpired:os.killpg(child.pid,signal.SIGKILL);child.wait()
                r=None

        text=path.read_text();entry={'client':name,'exit':None if r is None else r.returncode,'read_marker':('ORCHARD-742' in text) if not a.workflow else ('REFLECT-PORTABLE-OK' in text),'sandbox':'codex workspace-write' if name=='codex' and a.codex_native_sandbox else 'external macOS test sandbox','seconds':round(time.monotonic()-started,2)}
        report.append(entry);print(json.dumps(entry),flush=True)
    (evidence/('workflows.json' if a.workflow else 'clients.json')).write_text(json.dumps(report,indent=2)+'\n')
finally:
    proxy.shutdown();proxy.server_close()
    engine.terminate()
    try:engine.wait(timeout=15)
    except subprocess.TimeoutExpired:engine.kill();engine.wait()
    out.close()
assert all(r['exit']==0 and r['read_marker'] for r in report),report
