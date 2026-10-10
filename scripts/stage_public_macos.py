#!/usr/bin/env python3
"""Create a fresh, state-free Mac signing copy; never modify the working preview."""
import argparse,hashlib,json,shutil
from pathlib import Path
from package_preview import selected

def stage(source,destination):
    if destination.exists():raise SystemExit('Choose a fresh staging destination.')
    manifest=json.loads((source/'manifest.json').read_text());destination.mkdir(parents=True)
    hashes={}
    for name,expected in manifest.items():
        if not selected(name,'macos'):continue
        src=source/name
        if not src.is_file() or src.is_symlink():raise SystemExit('Invalid source: '+name)
        with src.open('rb')as f:
            if hashlib.file_digest(f,'sha256').hexdigest()!=expected:raise SystemExit('Integrity mismatch: '+name)
        dst=destination/name;dst.parent.mkdir(parents=True,exist_ok=True);shutil.copy2(src,dst);hashes[name]=expected
    (destination/'manifest.json').write_text(json.dumps(hashes,indent=2)+'\n')
if __name__=='__main__':
    p=argparse.ArgumentParser(description=__doc__);p.add_argument('--source',type=Path,required=True);p.add_argument('--destination',type=Path,required=True);a=p.parse_args();stage(a.source.resolve(),a.destination.resolve())
