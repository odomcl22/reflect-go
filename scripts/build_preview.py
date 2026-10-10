#!/usr/bin/env python3
"""Build the next preview without modifying preserved phase packages or models."""
import argparse
import concurrent.futures
import hashlib
import json
import os
from pathlib import Path
import plistlib
import re
import shutil
import subprocess
import sys

ROOT = Path(__file__).resolve().parents[1]
TARGETS = ['macos-arm64', 'macos-x64', 'windows-x64', 'windows-arm64', 'linux-x64', 'linux-arm64']
PREVIEW_VERSION = '0.4.8'
PREVIEW_BUILD = '48'
PREVIEW_FOLDER = 'Phase 4.8'

def preview_bundle_identifier(version=PREVIEW_VERSION):
    """Preserved previews must not compete for one macOS application identity."""
    if not re.fullmatch(r'\d+\.\d+\.\d+', version):
        raise ValueError('Preview version must contain major.minor.patch numbers.')
    return 'local.reflectgo.workstation.preview.v' + version.replace('.', '-')

def preview_info(source_info):
    info = dict(source_info)
    info.update(
        CFBundleIdentifier=preview_bundle_identifier(),
        CFBundleShortVersionString=PREVIEW_VERSION,
        CFBundleVersion=PREVIEW_BUILD,
        CFBundleIconFile='ReflectGo.icns',
        NSDocumentsFolderUsageDescription='Reflect Go reads its portable software, model weights and settings in the folder you open it from.',
    )
    return info

def matching_native_pids(process_output, executable):
    """Match the full executable path, not another preview with the same name."""
    executable = Path(executable).resolve()
    pids = []
    for line in process_output.splitlines():
        fields = line.strip().split(maxsplit=1)
        if len(fields) != 2 or not fields[0].isdigit():
            continue
        command = Path(fields[1])
        if command.is_absolute() and command.name == executable.name and command.resolve() == executable:
            pids.append(int(fields[0]))
    return pids

def require_native_app_stopped(destination):
    if sys.platform != 'darwin':
        return
    executable = destination/'Reflect Go.app/Contents/MacOS/ReflectGo'
    try:
        result = subprocess.run(['/bin/ps', '-axww', '-o', 'pid=,comm='],
                                check=True, capture_output=True, text=True)
    except (OSError, subprocess.CalledProcessError) as error:
        raise SystemExit('Could not check whether this Reflect Go preview is running. Close the target app and retry; no running app will be rebuilt.') from error
    pids = matching_native_pids(result.stdout, executable)
    if pids:
        raise SystemExit('Close Reflect Go in ' + str(destination) + ' before rebuilding it (PID ' + ', '.join(map(str, pids)) + '). The build will not replace a running app or stop it for you.')

def assemble(source, destination, refresh=False):
    if refresh and destination.name != PREVIEW_FOLDER:
        raise SystemExit('Refresh is restricted to ' + PREVIEW_FOLDER)
    if destination.exists() and not refresh:
        raise SystemExit('Destination exists; choose a fresh directory to preserve prior builds.')
    if not (source / 'manifest.json').is_file():
        raise SystemExit('A prepared Phase 4.7 software package is required.')
    # Replacing/signing the native binary while it is loaded changes the code
    # identity macOS sees for that running process. Refuse before any writes.
    require_native_app_stopped(destination)
    # Copy software only. Hard links reduce space for immutable runtime/client
    # packs; unlink all rebuilt files before writing. The app is a separate copy.
    if not refresh:
        shutil.copytree(source, destination, copy_function=os.link,
                    ignore=lambda path, names: [n for n in names if n in ('state', 'models', 'Models', 'Reflect Go.app', '.DS_Store') or n.startswith('._')])
        shutil.copytree(source/'Reflect Go.app', destination/'Reflect Go.app')
    compiler=ROOT/'.build/toolchains/go/bin/go'
    subprocess.run([sys.executable,str(ROOT/'scripts/build_icons.py')],check=True)
    env={**os.environ,'GOMODCACHE':str(ROOT/'.build/go-mod-cache'),'GOCACHE':str(ROOT/'.build/go-cache'),'GOTOOLCHAIN':'local','CGO_ENABLED':'0'}
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
    launcher=destination/'START Reflect Go.exe';launcher.unlink(missing_ok=True)
    subprocess.run([str(compiler),'build','-trimpath','-ldflags=-s -w -H=windowsgui','-o',str(launcher),'.'],cwd=ROOT/'Sources/WindowsLauncher',env={**env,'GOOS':'windows','GOARCH':'386'},check=True)
    temporary=ROOT/'.build/preview-native';temporary.mkdir(exist_ok=True)
    slices=[]
    for arch in ('arm64','x86_64'):
        out=temporary/arch;slices.append(str(out))
        subprocess.run(['/usr/bin/xcrun','swiftc','-parse-as-library','-O','-target',arch+'-apple-macosx13.3','-module-cache-path',str(ROOT/'.build/module-cache'),str(ROOT/'Sources/PortableWorkstation.swift'),'-o',str(out)],check=True)
    app=destination/'Reflect Go.app'
    # Compilation can take time; the user may have opened the target meanwhile.
    require_native_app_stopped(destination)
    subprocess.run(['/usr/bin/lipo','-create',*slices,'-output',str(app/'Contents/MacOS/ReflectGo')],check=True)
    resources=app/'Contents/Resources';resources.mkdir(exist_ok=True)
    shutil.copyfile(ROOT/'assets/icons/ReflectGo.icns',resources/'ReflectGo.icns')
    for name in ('ReflectGo.ico','ReflectGo.icns'):
        target=destination/name;target.unlink(missing_ok=True);shutil.copyfile(ROOT/'assets/icons'/name,target)
    info_path=app/'Contents/Info.plist';info=preview_info(plistlib.loads(info_path.read_bytes()));info_path.write_bytes(plistlib.dumps(info))
    subprocess.run(['/usr/bin/codesign','--force','--sign','-','--identifier',preview_bundle_identifier(),str(app)],check=True)
    for display,name in [('Codex','codex'),('Claude','claude'),('OpenCode','opencode')]:
        path=destination/('Connect '+display+'.command');path.unlink(missing_ok=True)
        path.write_text('#!/bin/sh\nROOT=$(CDPATH= cd -- "$(dirname -- "$0")" && pwd)\nexec /bin/sh "$ROOT/Reflect Go.sh" connect '+name+' --project "${1-}" --hold\n');path.chmod(0o755)
    for name in ('README.md','VALIDATION.md','ROADMAP.md','HARNESS_COMPATIBILITY.md'):
        target=destination/name;target.unlink(missing_ok=True)
        doc=ROOT/'docs'/({'README.md':'PREVIEW_0_4_8.md','VALIDATION.md':'PREVIEW_0_4_8_VALIDATION.md','ROADMAP.md':'ROADMAP.md','HARNESS_COMPATIBILITY.md':'HARNESS_COMPATIBILITY.md'}[name])
        if doc.exists():target.write_text(doc.read_text().replace('(PREVIEW_0_4_8_VALIDATION.md)', '(VALIDATION.md)'))
    # Runtime/client bytes are unchanged hard links from the verified source;
    # preserve their hashes and recalculate everything rebuilt above.
    for module,version in [('github.com/klauspost/compress','v1.18.0'),('github.com/pelletier/go-toml/v2','v2.2.4'),('github.com/tailscale/hujson','v0.0.0-20260727124030-b80ff77dac4f')]:
        license=ROOT/'.build/go-mod-cache'/(module+'@'+version)/'LICENSE'
        target=destination/'licenses'/(module.split('/')[-1]+'-LICENSE');target.parent.mkdir(exist_ok=True);target.unlink(missing_ok=True);shutil.copyfile(license,target)
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
    parser.add_argument('--source',type=Path,default=ROOT/'dist/Reflect Go/Phase 4.7')
    parser.add_argument('--destination',type=Path,default=ROOT/'dist/Reflect Go/Phase 4.8')
    parser.add_argument('--refresh',action='store_true',help='Rebuild an existing development preview; never use on preserved phases')
    args=parser.parse_args()
    assemble(args.source.resolve(),args.destination.resolve(),args.refresh)
