#!/usr/bin/env python3
"""Create a model-free or starter ZIP from prepared software, excluding private state.

Claude binaries are excluded from shareable archives until redistribution is
resolved. This command neither downloads components nor publishes a release.
"""
import argparse,hashlib,json,zipfile
from pathlib import Path
ROOT=Path(__file__).resolve().parents[1]

def package(bundle,output,starter=None):
    if output.exists():raise SystemExit('Output exists; choose a new archive path.')
    catalog=json.loads((ROOT/'Sources/ReflectGo/model-catalog.json').read_text())
    model=next(m for m in catalog if m['id']=='starter')
    if starter:
        if starter.name!=model['filename'] or starter.stat().st_size!=model['size']:raise SystemExit('Starter filename/size does not match the pinned model.')
        with starter.open('rb')as f:
            if hashlib.file_digest(f,'sha256').hexdigest()!=model['sha256']:raise SystemExit('Starter model integrity check failed.')
    manifest=json.loads((bundle/'manifest.json').read_text())
    members={}
    for p in bundle.rglob('*'):
        rel=p.relative_to(bundle).as_posix()
        if p.is_symlink():raise SystemExit('Package contains a symlink; inspect before archiving: '+rel)
        if not p.is_file() or rel.startswith(('state/','models/','Models/','harnesses/claude/')) or p.name.startswith('._') or p.name=='.DS_Store':continue
        if rel=='manifest.json':continue
        if rel not in manifest:raise SystemExit('Unmanifested package member: '+rel)
        with p.open('rb')as f:
            if hashlib.file_digest(f,'sha256').hexdigest()!=manifest[rel]:raise SystemExit('Integrity mismatch: '+rel)
        members[rel]=p
    output.parent.mkdir(parents=True,exist_ok=True)
    # Adjust portable pack metadata so missing Claude software is reported honestly.
    replacements={}
    for name in ('harnesses/installed.json',):
        if name in members:
            value=json.loads(members[name].read_text())
            value={k:v for k,v in value.items()if not k.startswith('claude/')}
            replacements[name]=(json.dumps(value,indent=2)+'\n').encode()
    clean={k:manifest[k]for k in members}
    for name,data in replacements.items():clean[name]=hashlib.sha256(data).hexdigest()
    if starter:clean['models/'+model['filename']]=model['sha256']
    prefix='Reflect Go/Phase 4.6/'
    with zipfile.ZipFile(output,'x',compression=zipfile.ZIP_DEFLATED,compresslevel=6,allowZip64=True)as z:
        z.writestr('Reflect Go/Models/','')
        for rel,path in sorted(members.items()):
            if rel in replacements:z.writestr(prefix+rel,replacements[rel])
            else:z.write(path,prefix+rel)
        z.writestr(prefix+'manifest.json',json.dumps(clean,indent=2)+'\n')
        z.writestr('Reflect Go/DOWNLOAD-README.txt','Reflect Go engineering preview. Start the platform launcher inside Phase 4.6.\nCodex and OpenCode packs are included; proprietary Claude binaries are excluded from this shareable archive.\nPrepare Claude from its official vendor package for a personal copy; see the source repository documentation.\n'+('The small starter model demonstrates chat, not coding reliability.\n'if starter else'No model weights included. Use Model library to download or import a model.\n'))
        if starter:z.write(starter,'Reflect Go/Models/'+model['filename'],compress_type=zipfile.ZIP_STORED)
    with output.open('rb')as f:digest=hashlib.file_digest(f,'sha256').hexdigest()
    output.with_suffix(output.suffix+'.sha256').write_text(digest+'  '+output.name+'\n')
    print(output)

if __name__=='__main__':
    p=argparse.ArgumentParser(description=__doc__);p.add_argument('--bundle',type=Path,default=ROOT/'dist/Reflect Go/Phase 4.6');p.add_argument('--output',type=Path,required=True);p.add_argument('--starter',type=Path);a=p.parse_args();package(a.bundle.resolve(),a.output.resolve(),a.starter.resolve()if a.starter else None)
