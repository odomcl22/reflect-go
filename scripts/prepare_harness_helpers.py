#!/usr/bin/env python3
"""Carry pinned search and Codex helper binaries instead of fetching at launch."""
import argparse,hashlib,json,shutil,tarfile,tempfile,urllib.request,zipfile
from pathlib import Path
from prepare_harnesses import ROOT,TARGETS,digest

LOCK=ROOT/'config/harness-helpers.lock.json'
def resolve():
    codex=json.loads((ROOT/'.build/phase45/codex-release.json').read_text())
    rg=json.loads((ROOT/'.build/phase45/ripgrep-release.json').read_text())
    result=[]
    for platform,(triple,_) in TARGETS.items():
        rgtriple=triple.replace('aarch64-unknown-linux-musl','aarch64-unknown-linux-gnu')
        filename='ripgrep-15.1.0-'+rgtriple+('.zip' if platform.startswith('windows') else '.tar.gz')
        selections=[(rg,filename,'harnesses/tools/'+platform+'/rg'+('.exe' if platform.startswith('windows') else ''),'rg')]
        names=['codex-app-server','codex-code-mode-host']
        if platform.startswith('windows'):names+=['codex-command-runner','codex-windows-sandbox-setup','codex-windows-sandbox-service']
        if platform.startswith('linux'):names+=['bwrap']
        for name in names:
            suffix='.exe' if platform.startswith('windows') else ''
            selections.append((codex,name+'-'+triple+suffix+'.tar.gz','harnesses/codex/'+platform+'/'+name+suffix,name))
        for release,filename,dest,name in selections:
            item=next(i for i in release['assets'] if i['name']==filename)
            expected=item.get('digest')
            if not expected:
                checksum=next(i for i in release['assets'] if i['name']==filename+'.sha256')
                expected='sha256:'+urllib.request.urlopen(checksum['browser_download_url']).read().decode().split()[0]
            assert expected.startswith('sha256:')
            result.append({'url':item['browser_download_url'],'sha256':expected.split(':')[1],'path':dest,'name':name,'platform':platform})
    LOCK.write_text(json.dumps(result,indent=2)+'\n');print('Pinned',len(result),'helpers')

def install(bundle):
    installed={};record=bundle/'harnesses/helpers.json'
    if record.exists():installed=json.loads(record.read_text())
    for info in json.loads(LOCK.read_text()):
        dest=bundle/info['path']
        if dest.exists() and installed.get(info['path'])==digest(dest):continue
        if dest.exists():raise ValueError('Unverified helper already exists '+str(dest))
        print('Preparing',info['path'],flush=True)
        with tempfile.TemporaryDirectory(dir=ROOT/'.build/phase45') as temporary:
            archive=Path(temporary)/'download'
            with urllib.request.urlopen(info['url'],timeout=90) as r,archive.open('wb') as out:shutil.copyfileobj(r,out)
            assert digest(archive)==info['sha256']
            dest.parent.mkdir(parents=True,exist_ok=True)
            if info['url'].endswith('.zip'):
                with zipfile.ZipFile(archive) as z:
                    item=next(n for n in z.namelist() if Path(n).name in [info['name'],info['name']+'.exe'])
                    with z.open(item) as src,dest.open('wb') as out:shutil.copyfileobj(src,out)
                    for item in z.namelist():
                        if Path(item).name in ['COPYING','LICENSE-MIT','UNLICENSE']:
                            license_path=bundle/'licenses/ripgrep'/Path(item).name;license_path.parent.mkdir(parents=True,exist_ok=True);license_path.write_bytes(z.read(item))
            else:
                with tarfile.open(archive,'r:gz') as t:
                    candidates=[i for i in t.getmembers() if i.isfile() and (Path(i.name).name==info['name'] or Path(i.name).name.startswith(info['name']+'-'))]
                    assert len(candidates)==1,(info['name'],[i.name for i in candidates])
                    with t.extractfile(candidates[0]) as src,dest.open('wb') as out:shutil.copyfileobj(src,out)
                    if info['name']=='rg':
                        for item in t.getmembers():
                            if item.isfile() and Path(item.name).name in ['COPYING','LICENSE-MIT','UNLICENSE']:
                                license_path=bundle/'licenses/ripgrep'/Path(item.name).name;license_path.parent.mkdir(parents=True,exist_ok=True);license_path.write_bytes(t.extractfile(item).read())
            dest.chmod(0o755)
        installed[info['path']]=digest(dest);record.write_text(json.dumps(installed,indent=2)+'\n')

if __name__=='__main__':
    p=argparse.ArgumentParser();p.add_argument('--resolve',action='store_true');p.add_argument('--bundle',type=Path);a=p.parse_args()
    if a.resolve:resolve()
    else:install(a.bundle.resolve())
