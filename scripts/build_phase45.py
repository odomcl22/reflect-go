#!/usr/bin/env python3
"""Compile Phase 4.5 front ends into an existing fresh, model-free Phase 4 copy."""
import argparse,concurrent.futures,json,os,plistlib,shutil,subprocess,tempfile
from pathlib import Path
from prepare_harnesses import ROOT,TARGETS,digest

def build(bundle, reuse_windows_launcher=False):
    assert bundle.name=='Phase 4.5' and (bundle/'runtime-targets.json').is_file()
    compiler=ROOT/'.build/toolchains/go/bin/go'
    env={**os.environ,'GOCACHE':str(ROOT/'.build/go-cache'),'GOTOOLCHAIN':'local','CGO_ENABLED':'0'}
    def compile_target(target):
        osname,arch=target.split('-');osname={'macos':'darwin'}.get(osname,osname);arch={'x64':'amd64'}.get(arch,arch)
        dest=bundle/'bin'/target;dest.mkdir(exist_ok=True,parents=True)
        for gui in ([False,True] if osname=='windows' else [False]):
            name=('reflectgo-window' if gui else 'reflectgo')+('.exe' if osname=='windows' else '')
            flags='-s -w'+(' -H=windowsgui' if gui else '')
            subprocess.run([str(compiler),'build','-trimpath','-ldflags='+flags,'-o',str(dest/name),'.'],cwd=ROOT/'Sources/ReflectGo',env={**env,'GOOS':osname,'GOARCH':arch},check=True)
        print('Built',target,flush=True)
    with concurrent.futures.ThreadPoolExecutor(max_workers=3)as ex:list(ex.map(compile_target,TARGETS))
    if reuse_windows_launcher:
        # Explicit maintenance mode: retain only a manifest-verified launcher.
        expected=json.loads((bundle/'manifest.json').read_text())['START Reflect Go.exe']
        assert digest(bundle/'START Reflect Go.exe')==expected,'Existing Windows launcher failed integrity verification'
    else:
        toolchain=next((ROOT/'.build/toolchains/windows').glob('llvm-mingw-*'))
        subprocess.run([str(toolchain/'bin/i686-w64-mingw32-clang'),'-O2','-Wall','-Wextra','-Werror','-municode','-mwindows',str(ROOT/'Sources/WindowsStart.c'),'-lshell32','-luser32','-o',str(bundle/'START Reflect Go.exe')],check=True)
    with tempfile.TemporaryDirectory(dir=ROOT/'.build/phase45') as tmp:
        paths=[]
        for arch in ['arm64','x86_64']:
            out=Path(tmp)/arch;paths.append(str(out))
            subprocess.run(['/usr/bin/xcrun','swiftc','-parse-as-library','-O','-target',arch+'-apple-macosx13.3','-module-cache-path',str(ROOT/'.build/module-cache'),str(ROOT/'Sources/Workstation.swift'),'-o',str(out)],check=True)
        app=bundle/'Reflect Go.app';subprocess.run(['/usr/bin/lipo','-create',*paths,'-output',str(app/'Contents/MacOS/ReflectGo')],check=True)
        info_path=app/'Contents/Info.plist';info=plistlib.loads(info_path.read_bytes());info.update(CFBundleShortVersionString='0.4.5',CFBundleVersion='45');info_path.write_bytes(plistlib.dumps(info))
        shutil.copyfile(bundle/'bin/engine-host',app/'Contents/MacOS/engine-host')
        (app/'Contents/MacOS/engine-host').chmod(0o755)
        subprocess.run(['/usr/bin/codesign','--force','--sign','-',str(app/'Contents/MacOS/engine-host')],check=True)
        subprocess.run(['/usr/bin/codesign','--force','--sign','-',str(app)],check=True)
    subprocess.run(['/usr/bin/codesign','--force','--sign','-',str(bundle/'bin/engine-host')],check=True)
    advanced=bundle/'Advanced';advanced.mkdir(exist_ok=True)
    for cmdfile in list(bundle.glob('*.cmd')):
        content=cmdfile.read_text().replace('%~dp0bin\\','%~dp0..\\bin\\')
        (advanced/cmdfile.name).write_text(content);cmdfile.unlink()
    for display,name in [('Codex','codex'),('Claude','claude'),('OpenCode','opencode')]:
        text='#!/bin/sh\nROOT=$(CDPATH= cd -- "$(dirname -- "$0")" && pwd)\nPROJECT=${1-}\nif [ -z "$PROJECT" ]; then printf "Project folder: "; IFS= read -r PROJECT; fi\nexec /bin/sh "$ROOT/Reflect Go.sh" connect '+name+' --project "$PROJECT"\n'
        path=bundle/('Connect '+display+'.command');path.write_text(text);path.chmod(0o755)
    for name in ['codex-local-instructions.txt','claude-local-instructions.txt']:shutil.copyfile(ROOT/'config'/name,bundle/'config'/name)
    for name in ['harness-targets.lock.json','harness-helpers.lock.json']:shutil.copyfile(ROOT/'config'/name,bundle/name)
    for source,target in [('codex-license.txt','Codex-LICENSE'),('codex-notice.txt','Codex-NOTICE'),('opencode-license.txt','OpenCode-LICENSE'),('claude-license','Claude-Code-TERMS')]:shutil.copyfile(ROOT/'.build/phase45'/source,bundle/'licenses'/target)
    for source,target in [('PHASE_4_5_README.md','README.md'),('PHASE_4_5_VALIDATION.md','VALIDATION.md')]:
        if (ROOT/'docs'/source).exists():shutil.copyfile(ROOT/'docs'/source,bundle/target)
    hashes={}
    for path in sorted(bundle.rglob('*')):
        relative=path.relative_to(bundle).as_posix()
        if path.is_file() and not relative.startswith('state/') and relative!='manifest.json' and path.name!='.DS_Store':hashes[relative]=digest(path)
    original=json.loads((bundle.parent/'Phase 4/manifest.json').read_text())
    hashes.update({p:h for p,h in original.items() if p.startswith('models/')})
    (bundle/'manifest.json').write_text(json.dumps(hashes,indent=2)+'\n')
    print('Built and checksummed Phase 4.5',flush=True)

if __name__=='__main__':
    p=argparse.ArgumentParser(description=__doc__);p.add_argument('--bundle',type=Path,default=ROOT/'dist/Reflect Go/Phase 4.5');p.add_argument('--reuse-windows-launcher',action='store_true',help='Keep the unchanged, manifest-verified native Windows entry point');a=p.parse_args();build(a.bundle.resolve(),reuse_windows_launcher=a.reuse_windows_launcher)
