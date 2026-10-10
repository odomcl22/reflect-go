#!/usr/bin/env python3
"""Build the next preview without modifying preserved phase packages or models."""
import argparse
import concurrent.futures
import hashlib
import json
import os
from pathlib import Path
import plistlib
import shutil
import subprocess

ROOT = Path(__file__).resolve().parents[1]
TARGETS = ['macos-arm64', 'macos-x64', 'windows-x64', 'windows-arm64', 'linux-x64', 'linux-arm64']

def assemble(source, destination, refresh=False):
    if destination.exists() and not refresh:
        raise SystemExit('Destination exists; choose a fresh directory to preserve prior builds.')
    if not (source / 'manifest.json').is_file():
        raise SystemExit('A prepared Phase 4.6 software package is required.')
    # Copy software only. Hard links reduce space for immutable runtime/client
    # packs; unlink all rebuilt files before writing. The app is a separate copy.
    if not refresh:
        shutil.copytree(source, destination, copy_function=os.link,
                    ignore=lambda path, names: [n for n in names if n in ('state', 'models', 'Models', 'Reflect Go.app', '.DS_Store') or n.startswith('._')])
        shutil.copytree(source/'Reflect Go.app', destination/'Reflect Go.app')
    compiler=ROOT/'.build/toolchains/go/bin/go'
    env={**os.environ,'GOCACHE':str(ROOT/'.build/go-cache'),'GOTOOLCHAIN':'local','CGO_ENABLED':'0'}
    def compile_target(target):
        system,arch=target.split('-');system={'macos':'darwin'}.get(system,system);arch={'x64':'amd64'}.get(arch,arch)
        out=destination/'bin'/target;out.mkdir(parents=True,exist_ok=True)
        for gui in ([False,True] if system=='windows' else [False]):
            binary=out/(('reflectgo-window' if gui else 'reflectgo')+('.exe' if system=='windows' else ''))
            binary.unlink(missing_ok=True)
            subprocess.run([str(compiler),'build','-trimpath','-ldflags=-s -w'+(' -H=windowsgui' if gui else ''),'-o',str(binary),'.'],cwd=ROOT/'Sources/ReflectGo',env={**env,'GOOS':system,'GOARCH':arch},check=True)
        print('Built',target,flush=True)
    with concurrent.futures.ThreadPoolExecutor(max_workers=3) as pool:
        list(pool.map(compile_target,TARGETS))
    temporary=ROOT/'.build/preview-native';temporary.mkdir(exist_ok=True)
    slices=[]
    for arch in ('arm64','x86_64'):
        out=temporary/arch;slices.append(str(out))
        subprocess.run(['/usr/bin/xcrun','swiftc','-parse-as-library','-O','-target',arch+'-apple-macosx13.3','-module-cache-path',str(ROOT/'.build/module-cache'),str(ROOT/'Sources/PortableWorkstation.swift'),'-o',str(out)],check=True)
    app=destination/'Reflect Go.app'
    subprocess.run(['/usr/bin/lipo','-create',*slices,'-output',str(app/'Contents/MacOS/ReflectGo')],check=True)
    info_path=app/'Contents/Info.plist';info=plistlib.loads(info_path.read_bytes());info.update(CFBundleShortVersionString='0.4.7',CFBundleVersion='47',NSDocumentsFolderUsageDescription='Reflect Go reads its portable software, model weights and settings in the folder you open it from.');info_path.write_bytes(plistlib.dumps(info))
    subprocess.run(['/usr/bin/codesign','--force','--sign','-',str(app)],check=True)
    for display,name in [('Codex','codex'),('Claude','claude'),('OpenCode','opencode')]:
        path=destination/('Connect '+display+'.command');path.unlink(missing_ok=True)
        path.write_text('#!/bin/sh\nROOT=$(CDPATH= cd -- "$(dirname -- "$0")" && pwd)\nexec /bin/sh "$ROOT/Reflect Go.sh" connect '+name+' --project "${1-}" --hold\n');path.chmod(0o755)
    for name in ('README.md','VALIDATION.md','ROADMAP.md','HARNESS_COMPATIBILITY.md'):
        target=destination/name;target.unlink(missing_ok=True)
        doc=ROOT/'docs'/({'README.md':'PREVIEW_0_4_7.md','VALIDATION.md':'PREVIEW_0_4_7_VALIDATION.md','ROADMAP.md':'ROADMAP.md','HARNESS_COMPATIBILITY.md':'HARNESS_COMPATIBILITY.md'}[name])
        if doc.exists():target.write_text(doc.read_text().replace('(PREVIEW_0_4_7_VALIDATION.md)', '(VALIDATION.md)'))
    # Runtime/client bytes are unchanged hard links from the verified source;
    # preserve their hashes and recalculate everything rebuilt above.
    manifest=json.loads((source/'manifest.json').read_text())
    for path in destination.rglob('*'):
        if not path.is_file() or path.name=='manifest.json':continue
        relative=path.relative_to(destination).as_posix()
        if relative.startswith('state/'):continue
        old=source/relative
        if not old.exists() or not os.path.samefile(path,old):
            with path.open('rb') as f:manifest[relative]=hashlib.file_digest(f,'sha256').hexdigest()
    manifest={k:v for k,v in manifest.items() if not k.startswith('state/') and (k.startswith('models/') or (destination/k).is_file())}
    path=destination/'manifest.json';path.unlink();path.write_text(json.dumps(manifest,indent=2)+'\n')
    print('Preview ready:',destination,flush=True)

if __name__=='__main__':
    parser=argparse.ArgumentParser(description=__doc__)
    parser.add_argument('--source',type=Path,default=ROOT/'dist/Reflect Go/Phase 4.6')
    parser.add_argument('--destination',type=Path,default=ROOT/'dist/Reflect Go/Phase 4.7')
    parser.add_argument('--refresh',action='store_true',help='Rebuild an existing development preview; never use on preserved phases')
    args=parser.parse_args()
    if args.refresh and args.destination.name != 'Phase 4.7':raise SystemExit('Refresh is restricted to Phase 4.7')
    assemble(args.source.resolve(),args.destination.resolve(),args.refresh)
