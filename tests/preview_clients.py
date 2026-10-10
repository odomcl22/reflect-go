#!/usr/bin/env python3
"""Test the packaged client adapters without requiring a selected project."""
import argparse,os,subprocess,json,signal
from pathlib import Path
p=argparse.ArgumentParser();p.add_argument('bundle',type=Path);p.add_argument('--client',choices=['codex','claude','opencode'],required=True);a=p.parse_args();root=a.bundle.resolve();exe=root/'bin/macos-arm64/reflectgo';log=root/'state'/('preview-'+a.client+'.log')
with log.open('w') as f:
 try:
  process=subprocess.Popen([str(exe),'connect',a.client,'--task','Reply with exactly: local connection works. Do not use tools.'],stdout=f,stderr=subprocess.STDOUT,start_new_session=True)
  code=process.wait(timeout=90)
 except subprocess.TimeoutExpired:
  os.killpg(process.pid,signal.SIGTERM)
  try:process.wait(timeout=5)
  except subprocess.TimeoutExpired:os.killpg(process.pid,signal.SIGKILL);process.wait()
  code=124
print(a.client,'exit',code,flush=True)
text=log.read_text(errors='replace');print(text[-4500:]);raise SystemExit(code)
