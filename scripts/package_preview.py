#!/usr/bin/env python3
"""Create platform-specific public ZIPs from a verified build, with no private state.

Claude binaries are excluded pending redistribution permission. An optional pinned
starter model provides basic chat. No uploads are performed by this script.
"""
import argparse, hashlib, json, re, zipfile
from pathlib import Path
ROOT=Path(__file__).resolve().parents[1]

def public_build_metadata(value):
    """Build flags are provenance, not a reason to publish a builder's home path."""
    if isinstance(value, str):
        value = re.sub(r'/(?:Users|home)/[^/\s]+', '<BUILD_HOME>', value)
        return re.sub(r'[A-Za-z]:\\Users\\[^\\\s]+', '<BUILD_HOME>', value)
    if isinstance(value, list):return [public_build_metadata(v) for v in value]
    if isinstance(value, dict):return {k:public_build_metadata(v) for k,v in value.items()}
    return value

def selected(rel, platform):
    parts=Path(rel).parts
    if parts[0] in ('state','models','Models') or rel.startswith('harnesses/claude/'):
        return False
    if not platform:return True
    if parts[0] in ('bin','runtime') and len(parts)>1:return parts[1].startswith(platform+'-')
    if parts[0]=='harnesses' and len(parts)>2:
        return any(p.startswith(platform+'-') for p in parts[1:])
    if parts[0]=='Reflect Go.app' or rel.endswith('.command') or rel=='ReflectGo.icns':return platform=='macos'
    if parts[0]=='Advanced' or rel.endswith(('.exe','.cmd','.bat','.ps1')) or rel=='ReflectGo.ico':return platform=='windows'
    if rel.endswith('.sh'):return platform!='windows'
    return True

def package(bundle,output,starter=None,platform=None):
    if output.exists():raise SystemExit('Output exists; choose a new archive path.')
    catalog=json.loads((ROOT/'Sources/ReflectGo/model-catalog.json').read_text())
    model=next(m for m in catalog if m['id']=='starter')
    if starter:
        if starter.name!=model['filename'] or starter.stat().st_size!=model['size']:raise SystemExit('Starter filename/size does not match the pinned model.')
        with starter.open('rb')as f:
            if hashlib.file_digest(f,'sha256').hexdigest()!=model['sha256']:raise SystemExit('Starter model integrity check failed.')
    manifest=json.loads((bundle/'manifest.json').read_text());members={}
    for p in bundle.rglob('*'):
        rel=p.relative_to(bundle).as_posix()
        if not selected(rel,platform):continue
        if p.is_symlink():raise SystemExit('Package contains a symlink; inspect before archiving: '+rel)
        if not p.is_file() or p.name.startswith('._') or p.name in ('.DS_Store','manifest.json'):continue
        if rel not in manifest:raise SystemExit('Unmanifested package member: '+rel)
        with p.open('rb')as f:
            if hashlib.file_digest(f,'sha256').hexdigest()!=manifest[rel]:raise SystemExit('Integrity mismatch: '+rel)
        members[rel]=p
    replacements={}
    for name, path in members.items():
        if name.endswith('/BUILD.json'):
            replacements[name]=(json.dumps(public_build_metadata(json.loads(path.read_text())),indent=2)+'\n').encode()
    for name in ('harnesses/installed.json','runtime-targets.json'):
        if name in members:
            value=json.loads(members[name].read_text())
            value={k:v for k,v in value.items() if not k.startswith('claude/') and (not platform or v.get('platform',k.split('/')[-1]).startswith(platform+'-'))}
            replacements[name]=(json.dumps(value,indent=2)+'\n').encode()
    name='harnesses/helpers.json'
    if name in members:
        value=json.loads(members[name].read_text())
        replacements[name]=(json.dumps({k:v for k,v in value.items() if k in members},indent=2)+'\n').encode()
    name='config/portableai.json'
    if name in members:
        value=json.loads(members[name].read_text());value['models_directory']='Models'
        # A public archive cannot default to a model it does not include.
        value.pop('provider_model',None)
        replacements[name]=(json.dumps(value,indent=2)+'\n').encode()
    clean={k:manifest[k]for k in members}
    for name,data in replacements.items():clean[name]=hashlib.sha256(data).hexdigest()
    if starter:clean['models/'+model['filename']]=model['sha256']
    signing=json.loads(members['SIGNING.json'].read_text()) if 'SIGNING.json' in members else {}
    signature=('Mac binaries are Developer ID signed and notarized.\n' if platform=='macos' and signing.get('notarization_status')=='Accepted' else 'Preview binaries do not carry trusted publisher verification; see release notes.\n')
    readme=('Reflect Go 0.4.8 preview. Extract the entire ZIP before opening.\n'
            'Mac: Reflect Go.app. Windows: START Reflect Go.exe. Linux: sh "Reflect Go.sh".\n'
            'Keep the app/exe beside bin, runtime, harnesses, config and Models.\n'
            'Codex and OpenCode CLI packs included. Proprietary Claude CLI binaries excluded.\n'
            'Installed desktop apps connect through More apps; preview qualification applies.\n'
            'Quit Reflect Go before unplugging. Connected desktop apps restart with previous settings.\n'
            +signature
            +('Includes a small demonstration model for simple chat; desktop clients need a different 49K-context-capable model.\n'if starter else'No weights included. Use Model library to download/import a GGUF.\n'))
    clean['DOWNLOAD-README.txt']=hashlib.sha256(readme.encode()).hexdigest()
    output.parent.mkdir(parents=True,exist_ok=True)
    prefix='Reflect Go/'
    with zipfile.ZipFile(output,'x',compression=zipfile.ZIP_DEFLATED,compresslevel=6,allowZip64=True)as z:
        z.writestr(prefix+'Models/','')
        for rel,path in sorted(members.items()):
            if rel in replacements:z.writestr(prefix+rel,replacements[rel])
            else:z.write(path,prefix+rel)
        z.writestr(prefix+'manifest.json',json.dumps(clean,indent=2)+'\n')
        z.writestr(prefix+'DOWNLOAD-README.txt',readme)
        if starter:z.write(starter,prefix+'Models/'+model['filename'],compress_type=zipfile.ZIP_STORED)
    with output.open('rb')as f:digest=hashlib.file_digest(f,'sha256').hexdigest()
    output.with_suffix(output.suffix+'.sha256').write_text(digest+'  '+output.name+'\n')
    print(output,flush=True)

if __name__=='__main__':
    p=argparse.ArgumentParser(description=__doc__)
    p.add_argument('--bundle',type=Path,default=ROOT/'dist/Reflect Go/Phase 4.8');p.add_argument('--output',type=Path,required=True)
    p.add_argument('--starter',type=Path);p.add_argument('--platform',choices=('macos','windows','linux'))
    a=p.parse_args();package(a.bundle.resolve(),a.output.resolve(),a.starter.resolve()if a.starter else None,a.platform)
