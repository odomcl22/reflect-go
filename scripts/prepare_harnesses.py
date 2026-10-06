#!/usr/bin/env python3
"""Resolve or install pinned native CLI packs, without executing upstream installers."""
import argparse, base64, hashlib, json, os, shutil, tarfile, tempfile, urllib.request, zipfile
from pathlib import Path

ROOT = Path(__file__).resolve().parents[1]
LOCK = ROOT / 'config/harness-targets.lock.json'
TARGETS = {'macos-arm64': ('aarch64-apple-darwin','darwin-arm64'), 'macos-x64': ('x86_64-apple-darwin','darwin-x64'),
           'windows-x64': ('x86_64-pc-windows-msvc','win32-x64'), 'windows-arm64': ('aarch64-pc-windows-msvc','win32-arm64'),
           'linux-x64': ('x86_64-unknown-linux-musl','linux-x64'), 'linux-arm64': ('aarch64-unknown-linux-musl','linux-arm64')}

def fetch(url):
    with urllib.request.urlopen(url, timeout=90) as r: return r.read()

def resolve():
    releases = {n: json.loads((ROOT / '.build/phase45' / (n+'-release.json')).read_text()) for n in ['codex','opencode']}
    lock = {'schema':1, 'versions':{'codex':'0.160.0','opencode':'1.18.34','claude':'2.1.289'}, 'packs':{}}
    for platform, (triple, npm) in TARGETS.items():
        for name in ['codex','opencode','claude']:
            if name == 'claude':
                package = '@anthropic-ai/claude-code-'+npm
                d = json.loads(fetch('https://registry.npmjs.org/'+package+'/2.1.289'))
                info = {'url':d['dist']['tarball'],'integrity':d['dist']['integrity'], 'license':'Proprietary; personal copy; excluded from public archive', 'unpacked_bytes':d['dist'].get('unpackedSize')}
            else:
                if name == 'codex': filename = 'codex-'+triple+('.exe' if platform.startswith('windows') else '')+'.tar.gz'
                else:
                    system = platform.replace('macos','darwin')
                    baseline = '-baseline' if system.endswith('x64') else ''
                    filename = 'opencode-'+system+baseline+('.zip' if not platform.startswith('linux') else '.tar.gz')
                a = next(a for a in releases[name]['assets'] if a['name']==filename)
                assert a['digest'].startswith('sha256:')
                info = {'url':a['browser_download_url'],'sha256':a['digest'].split(':')[1], 'license':'Apache-2.0' if name=='codex' else 'MIT', 'download_bytes':a['size']}
            lock['packs'][name+'/'+platform] = info
    LOCK.write_text(json.dumps(lock,indent=2)+'\n')
    print('Pinned',len(lock['packs']),'packs')

def digest(path):
    h=hashlib.sha256()
    with path.open('rb') as f:
        for block in iter(lambda:f.read(1024*1024), b''): h.update(block)
    return h.hexdigest()

def install(bundle, platforms):
    (ROOT/'.build/phase45').mkdir(parents=True,exist_ok=True)
    lock=json.loads(LOCK.read_text()); records={}
    record_path=bundle/'harnesses/installed.json'
    if record_path.exists(): records=json.loads(record_path.read_text())
    for key, info in lock['packs'].items():
        name,platform=key.split('/')
        if platform not in platforms:continue
        folder=bundle/'harnesses'/name/platform
        exe=folder/(name+('.exe' if platform.startswith('windows') else ''))
        if key in records and exe.exists() and digest(exe)==records[key]['sha256']:continue
        if folder.exists():raise RuntimeError('Refusing to overwrite unverified pack '+key)
        print('Downloading',key,flush=True)
        with tempfile.TemporaryDirectory(prefix='harness-',dir=ROOT/'.build/phase45') as temporary:
            tmp=Path(temporary);archive=tmp/'download'
            with urllib.request.urlopen(info['url'],timeout=90) as r, archive.open('wb') as out:shutil.copyfileobj(r,out,1024*1024)
            if 'sha256' in info:assert digest(archive)==info['sha256'],key+' checksum mismatch'
            else:
                algorithm,expected=info['integrity'].split('-',1);h=hashlib.new(algorithm)
                with archive.open('rb') as f:
                    for block in iter(lambda:f.read(1024*1024),b''):h.update(block)
                assert base64.b64encode(h.digest()).decode()==expected,key+' checksum mismatch'
            stage=tmp/'pack';stage.mkdir()
            # Copy only regular members, flattening the one native CLI; retain licenses and helper files.
            if info['url'].endswith('.zip'):
                with zipfile.ZipFile(archive) as z:
                    for item in z.infolist():
                        if item.is_dir():continue
                        p=Path(item.filename)
                        if p.is_absolute() or '..' in p.parts or '\\' in item.filename or (item.external_attr>>16)&0o170000==0o120000:raise ValueError('Unsafe archive member')
                        dest=stage/p;dest.parent.mkdir(parents=True,exist_ok=True)
                        with z.open(item) as src,dest.open('wb') as out:shutil.copyfileobj(src,out)
            else:
                with tarfile.open(archive,'r:gz') as t:
                    for item in t:
                        p=Path(item.name)
                        if p.is_absolute() or '..' in p.parts or '\\' in item.name or item.issym() or item.islnk():raise ValueError('Unsafe archive member')
                        if not item.isfile():continue
                        dest=stage/p;dest.parent.mkdir(parents=True,exist_ok=True)
                        with t.extractfile(item) as src,dest.open('wb') as out:shutil.copyfileobj(src,out)
            candidates=[p for p in stage.rglob('*') if p.is_file() and (p.name==name or p.name==name+'.exe' or (name=='codex' and p.name.startswith('codex-')))]
            assert len(candidates)==1,(key,[str(p) for p in candidates])
            binary=candidates[0];target=stage/(name+('.exe' if platform.startswith('windows') else ''))
            if binary!=target:shutil.move(binary,target)
            target.chmod(0o755)
            folder.parent.mkdir(parents=True,exist_ok=True);shutil.move(stage,folder)
        records[key]={'version':lock['versions'][name], 'path':exe.relative_to(bundle).as_posix(), 'sha256':digest(exe), 'license':info['license']}
        record_path.parent.mkdir(parents=True,exist_ok=True);record_path.write_text(json.dumps(records,indent=2)+'\n')
        print('Ready',key,exe.stat().st_size,flush=True)
    # Restoring a personal pack into an archive-restored bundle must also keep
    # its integrity index consistent; no compiler is needed for this operation.
    manifest_path=bundle/'manifest.json'
    if manifest_path.exists():
        hashes=json.loads(manifest_path.read_text())
        for path in sorted((bundle/'harnesses').rglob('*')):
            if path.is_file():hashes[path.relative_to(bundle).as_posix()]=digest(path)
        manifest_path.write_text(json.dumps(hashes,indent=2)+'\n')

if __name__=='__main__':
    p=argparse.ArgumentParser(description=__doc__);p.add_argument('--resolve',action='store_true');p.add_argument('--bundle',type=Path);p.add_argument('--platforms',nargs='+',default=list(TARGETS));a=p.parse_args()
    if a.resolve:resolve()
    elif a.bundle:install(a.bundle.resolve(),a.platforms)
    else:p.error('Choose --resolve or --bundle')
