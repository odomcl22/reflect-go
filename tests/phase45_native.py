#!/usr/bin/env python3
"""Run the real Mac controller on a disposable APFS clone of Phase 4.5."""
from pathlib import Path
import shutil,socket,subprocess,time
root=Path(__file__).resolve().parents[1]
phase=root/'dist/Reflect Go/.phase45-native-test'
assert not phase.exists(),'Refusing to overwrite existing test phase'
subprocess.run(['/bin/cp','-cR',str(root/'dist/Reflect Go/Phase 4.5'),str(phase)],check=True)
shutil.rmtree(phase/'state');(phase/'state').mkdir();(phase/'state/workstation.json').write_text('corrupt settings test')
evidence=root/'docs/evidence/phase-4.5/native-controller.txt'
try:
 with evidence.open('w') as out:
  child=subprocess.Popen([str(root/'.build/phase45/workstation-acceptance'),str(phase),'--crash'],stdout=out,stderr=subprocess.STDOUT,env={'PATH':'/usr/bin:/bin:/usr/sbin:/sbin','LANG':'en_US.UTF-8'})
  try:code=child.wait(timeout=220)
  except subprocess.TimeoutExpired:child.kill();child.wait();raise
 text=evidence.read_text();print(text,flush=True)
 assert code==-9 and 'CRASHING TEST OWNER' in text,(code,text)
 deadline=time.monotonic()+15
 while time.monotonic()<deadline:
  with socket.socket() as check:
   check.setsockopt(socket.SOL_SOCKET,socket.SO_REUSEADDR,1)
   try:check.bind(('127.0.0.1',18876));break
   except OSError:time.sleep(.2)
 else:raise RuntimeError('Controller crash left its engine listener running')
 with evidence.open('a') as out:out.write('PASS: force-killed Phase 4.5 controller released its 4B engine listener\n')
 print('PASS: crash cleanup',flush=True)
finally:
 # Only the disposable clone created above; models are sibling shared files.
 shutil.rmtree(phase)
