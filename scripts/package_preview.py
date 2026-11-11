#!/usr/bin/env python3
"""Create platform-specific public ZIPs from a verified build, with no private state.

Claude binaries are excluded pending redistribution permission. An optional pinned
starter model provides basic chat. No uploads are performed by this script.
"""
import argparse, hashlib, json, re, zipfile
from pathlib import Path
ROOT=Path(__file__).resolve().parents[1]
SUPPORT='Reflect Go Files/'

def launcher_member(rel):
    return rel.startswith('Reflect Go.app/') or rel=='START Reflect Go.exe'

def archive_member(rel):
    if rel=='START Reflect Go.exe':return 'Reflect Go.exe'
    if rel.startswith('Reflect Go.app/'):return rel
    return SUPPORT+rel

def linux_desktop():
    # Keep the entry location in a separate argument, never in shell code.
    command='exec sh "$(dirname -- "$1")/Reflect Go Files/Reflect Go.sh"'
    quoted='"'+command.replace('\\','\\\\').replace('"','\\"').replace('$','\\$').replace('`','\\`')+'"'
    # The desktop value unescapes backslashes before parsing quoted arguments.
    quoted=quoted.replace('\\','\\\\')
    return '[Desktop Entry]\nType=Application\nName=Reflect Go\nComment=Your portable AI workstation\nIcon=applications-development\nTerminal=false\nExec=sh -c '+quoted+' ReflectGo %k\nCategories=Development;\n'

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

def package(bundle,output,starter=None,platform=None,version='0.4.10'):
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
    # The support manifest is relative to Reflect Go Files. Top-level native
    # launchers are covered by the ZIP checksum and Apple's app signature.
    clean={k:manifest[k]for k in members if not launcher_member(k)}
    for name,data in replacements.items():
        if not launcher_member(name):clean[name]=hashlib.sha256(data).hexdigest()
    # Claude is downloaded by each user from Anthropic's pinned native package;
    # no proprietary executable is redistributed in the public archive.
    claude_lock=json.loads((ROOT/'config/harness-targets.lock.json').read_text())
    # Keep all six download records, even in a platform-specific archive, so a
    # user can prepare their own drive for another computer without a rebuild.
    claude_lock['packs']={k:v for k,v in claude_lock['packs'].items() if k.startswith('claude/')}
    claude_lock['versions']={'claude':claude_lock['versions']['claude']}
    additions={
        'LICENSE':(ROOT/'LICENSE').read_bytes(),
        'tools/add_claude_cli.py':(ROOT/'scripts/add_claude_cli.py').read_bytes(),
        'tools/prepare_harnesses.py':(ROOT/'scripts/prepare_harnesses.py').read_bytes(),
        'config/claude-cli.lock.json':(json.dumps(claude_lock,indent=2)+'\n').encode(),
    }
    # Documentation is assembled from authoritative sources, just like embedded
    # UI assets. It may be finalized after code signing without changing code.
    doc_stem='PREVIEW_'+version.replace('.','_')
    for name,source in {
        'README.md':doc_stem+'.md', 'VALIDATION.md':doc_stem+'_VALIDATION.md',
        'ROADMAP.md':'ROADMAP.md', 'HARNESS_COMPATIBILITY.md':'HARNESS_COMPATIBILITY.md',
        'PORTABLE_CLIS.md':'PORTABLE_CLIS.md', 'LM_STUDIO_SELECTOR_REVIEW.md':'LM_STUDIO_SELECTOR_REVIEW.md',
    }.items():
        data=(ROOT/'docs'/source).read_text().replace('('+doc_stem+'_VALIDATION.md)','(VALIDATION.md)').encode()
        if name in members:
            replacements[name]=data;clean[name]=hashlib.sha256(data).hexdigest()
        else:additions[name]=data
    for name,data in additions.items():clean[name]=hashlib.sha256(data).hexdigest()
    if starter:clean['models/'+model['filename']]=model['sha256']
    signing=json.loads(members['SIGNING.json'].read_text()) if 'SIGNING.json' in members else {}
    signature=('Mac binaries are Developer ID signed and notarized.\n' if platform=='macos' and signing.get('notarization_status')=='Accepted' else 'Preview binaries do not carry trusted publisher verification; see release notes.\n')
    readme=(f'Reflect Go {version} preview. Extract the entire ZIP before opening.\n'
            'Mac: Reflect Go.app. Windows: Reflect Go.exe. Linux: Reflect Go.desktop.\n'
            'Keep the launcher beside Reflect Go Files. Models, clients and settings stay inside that folder.\n'
            'Linux fallback: sh "Reflect Go Files/Reflect Go.sh". Your desktop may require Allow Launching.\n'
            'Codex and OpenCode CLI packs included. To add Claude Code to your own drive,\n'
            'open Reflect Go and click Get Claude Code. No Python or Node installation needed.\n'
            'That optional one-time download requires internet and uses Anthropic directly.\n'
            'Installed desktop apps connect through More apps; preview qualification applies.\n'
            'Quit Reflect Go before unplugging. Connected desktop apps restart with previous settings.\n'
            +signature
            +('Includes a small demonstration model for simple chat; desktop clients need a different 49K-context-capable model.\n'if starter else'No weights included. Use Model library to download/import a GGUF.\n'))
    clean['DOWNLOAD-README.txt']=hashlib.sha256(readme.encode()).hexdigest()
    output.parent.mkdir(parents=True,exist_ok=True)
    prefix='Reflect Go/'
    with zipfile.ZipFile(output,'x',compression=zipfile.ZIP_DEFLATED,compresslevel=6,allowZip64=True)as z:
        z.writestr(prefix+SUPPORT+'Models/','')
        for rel,path in sorted(members.items()):
            if rel in replacements:z.writestr(prefix+archive_member(rel),replacements[rel])
            else:z.write(path,prefix+archive_member(rel))
        for name,data in additions.items():z.writestr(prefix+SUPPORT+name,data)
        z.writestr(prefix+SUPPORT+'manifest.json',json.dumps(clean,indent=2)+'\n')
        z.writestr(prefix+SUPPORT+'DOWNLOAD-README.txt',readme)
        if platform=='linux':
            info=zipfile.ZipInfo(prefix+'Reflect Go.desktop');info.create_system=3;info.external_attr=0o100755<<16
            z.writestr(info,linux_desktop())
        if starter:z.write(starter,prefix+SUPPORT+'Models/'+model['filename'],compress_type=zipfile.ZIP_STORED)
    with output.open('rb')as f:digest=hashlib.file_digest(f,'sha256').hexdigest()
    output.with_suffix(output.suffix+'.sha256').write_text(digest+'  '+output.name+'\n')
    print(output,flush=True)

if __name__=='__main__':
    p=argparse.ArgumentParser(description=__doc__)
    p.add_argument('--bundle',type=Path,default=ROOT/'dist/Reflect Go/Phase 4.10');p.add_argument('--output',type=Path,required=True)
    p.add_argument('--starter',type=Path);p.add_argument('--platform',choices=('macos','windows','linux'))
    p.add_argument('--version',choices=('0.4.9','0.4.10'),default='0.4.10')
    a=p.parse_args();package(a.bundle.resolve(),a.output.resolve(),a.starter.resolve()if a.starter else None,a.platform,a.version)
