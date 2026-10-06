#!/usr/bin/env python3
"""Copy new phase software to an existing collection; never copy weights or state."""
import argparse
import hashlib
import json
from pathlib import Path
import shutil
import subprocess
import uuid

ROOT = Path(__file__).resolve().parents[1]

def digest(path):
    result = hashlib.sha256()
    with path.open('rb') as stream:
        for block in iter(lambda: stream.read(8 * 1024 * 1024), b''): result.update(block)
    return result.hexdigest()

def deploy(phase, collection):
    source = ROOT / 'dist/Reflect Go' / f'Phase {phase}'
    target = collection / source.name
    if not collection.is_dir() or not (collection / 'Models').is_dir():
        raise ValueError('Choose an existing collection with its shared Models folder')
    if target.exists(): raise ValueError(f'Refusing to overwrite {target}')
    manifest = json.loads((source / 'manifest.json').read_text())
    for name, expected in manifest.items():
        relative = Path(name)
        if relative.is_absolute() or '..' in relative.parts: raise ValueError('Unsafe manifest path')
        path = collection / 'Models' / relative.name if relative.parts[0] == 'models' else source / relative
        if digest(path) != expected: raise ValueError(f'Integrity mismatch: {path}')
    staging = collection / f'.{source.name}-copy-{uuid.uuid4().hex[:8]}'
    staging.mkdir()
    for path in sorted(source.rglob('*')):
        rel = path.relative_to(source)
        if any(part in ('models', 'Models', 'state', 'releases', '.DS_Store') or part.startswith('._') for part in rel.parts): continue
        if path.is_symlink(): raise ValueError(f'Symlink needs inspection: {path}')
        if path.suffix.lower() == '.gguf': raise ValueError('Weights must stay in shared Models')
        dest = staging / rel
        if path.is_dir(): dest.mkdir(exist_ok=True)
        elif path.is_file():
            # Byte copy avoids extended attributes/AppleDouble signature damage.
            shutil.copyfile(path, dest)
            dest.chmod(path.stat().st_mode & 0o777)
    for name, expected in manifest.items():
        if Path(name).parts[0] != 'models' and digest(staging / name) != expected:
            raise ValueError(f'Copy verification failed: {name}')
    subprocess.run(['/usr/bin/codesign', '--verify', '--deep', '--strict', str(staging / 'Reflect Go.app')], check=True)
    staging.rename(target)
    shutil.copyfile(ROOT / 'README.md', collection / 'README.md')
    print(f'Deployed {target}; no models, credentials, state, or backup archives copied.')

if __name__ == '__main__':
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument('--phase', choices=['3.1', '4'], default='3.1')
    parser.add_argument('--collection', type=Path, required=True)
    args = parser.parse_args()
    deploy(args.phase, args.collection.resolve())
