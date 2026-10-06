#!/usr/bin/env python3
"""Run the actual Swift Workstation controller on an isolated software-only phase."""
from pathlib import Path
import argparse
import shutil
import socket
import subprocess
import tempfile
import time

ROOT = Path(__file__).resolve().parents[1]
parser = argparse.ArgumentParser(); parser.add_argument('--phase', default='3.1'); parser.add_argument('--binary', type=Path, default=ROOT / '.build/workstation-acceptance'); args = parser.parse_args()
BUNDLE = ROOT / 'dist/Reflect Go' / ('Phase ' + args.phase)
with tempfile.TemporaryDirectory(prefix='.phase31-test-', dir=BUNDLE.parent) as temporary:
    phase = Path(temporary)
    shutil.copytree(BUNDLE, phase, dirs_exist_ok=True, ignore=shutil.ignore_patterns('state', 'models', '._*'))
    (phase / 'state').mkdir()
    (phase / 'state/workstation.json').write_text('corrupt settings test')
    child = subprocess.Popen([str(args.binary.resolve()), str(phase), '--crash'], stdout=subprocess.PIPE, stderr=subprocess.STDOUT, text=True)
    try:
        output, _ = child.communicate(timeout=220)
        print(output, end='')
        if child.returncode != -9 or 'CRASHING TEST OWNER' not in output:
            raise RuntimeError(f'Acceptance exited {child.returncode}')
        deadline = time.monotonic() + 12
        while time.monotonic() < deadline:
            with socket.socket() as check:
                check.setsockopt(socket.SOL_SOCKET, socket.SO_REUSEADDR, 1)
                try: check.bind(('127.0.0.1', 18876)); break
                except OSError: time.sleep(.2)
        else: raise RuntimeError('Force-killed app left its engine listener running')
        print('PASS: force-killed Workstation releases real 4B engine listener')
    finally:
        if child.poll() is None: child.kill(); child.wait()
