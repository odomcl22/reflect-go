#!/usr/bin/env python3
"""Preserve source and one assembled phase in the project's only archive folder."""
import argparse
import hashlib
import io
import json
from pathlib import Path
import tarfile

ROOT = Path(__file__).resolve().parents[1]
SOURCE_ITEMS = ['Sources', 'launchers', 'config', 'scripts', 'tests', 'docs', 'README.md',
                'PortableAI.command', 'Start Provider.command', 'Connect Codex.command',
                'Connect Claude.command', 'VerifyPortability.command']
EXCLUDED = {'models', 'Models', 'state', '__pycache__', '.DS_Store'}


def allowed(path):
    return not any(part in EXCLUDED or part.startswith('._') for part in path.parts) and path.suffix.lower() != '.gguf'


def file_hash(path):
    value = hashlib.sha256()
    with path.open('rb') as stream:
        for block in iter(lambda: stream.read(1024 * 1024), b''):
            value.update(block)
    return value.hexdigest()


def archive_release(phase, label):
    if not label or any(c not in 'abcdefghijklmnopqrstuvwxyz0123456789-' for c in label):
        raise ValueError('Use a lowercase, hyphenated archive label')
    bundle = ROOT / 'dist/Reflect Go' / f'Phase {phase}'
    if not bundle.is_dir():
        raise ValueError(f'Missing assembled phase: {bundle}')
    target = ROOT / 'releases' / f'{label}.tar.gz'
    target.parent.mkdir(exist_ok=True)
    if target.exists() or target.with_suffix('.part').exists():
        raise ValueError(f'Archive exists; refusing overwrite: {target}')
    partial = target.with_suffix('.part')
    replacements = {}
    if phase == '4.5':
        packs = json.loads((bundle / 'harnesses/installed.json').read_text())
        replacements['harnesses/installed.json'] = (json.dumps(
            {k: v for k, v in packs.items() if not k.startswith('claude/')}, indent=2) + '\n').encode()
        manifest = json.loads((bundle / 'manifest.json').read_text())
        manifest = {k: v for k, v in manifest.items() if not k.startswith('harnesses/claude/')}
        manifest['harnesses/installed.json'] = hashlib.sha256(replacements['harnesses/installed.json']).hexdigest()
        replacements['manifest.json'] = (json.dumps(manifest, indent=2) + '\n').encode()
    with tarfile.open(partial, 'w:gz') as tar:
        for item in SOURCE_ITEMS:
            base = ROOT / item
            for path in ([base] if base.is_file() else sorted(base.rglob('*'))):
                rel = path.relative_to(ROOT)
                if path.is_file() and allowed(rel):
                    if path.is_symlink():
                        raise ValueError(f'Source symlink requires inspection: {rel}')
                    tar.add(path, arcname=str(Path('source') / rel), recursive=False)
        for path in sorted(bundle.rglob('*')):
            rel = path.relative_to(bundle)
            if phase == '4.5' and rel.as_posix().startswith('harnesses/claude/'):
                continue
            if path.is_file() and allowed(rel):
                if path.is_symlink():
                    raise ValueError(f'Bundle symlink requires inspection: {rel}')
                name = str(Path('bundle') / rel)
                if rel.as_posix() in replacements:
                    data = replacements[rel.as_posix()]
                    info = tar.gettarinfo(str(path), arcname=name)
                    info.size = len(data)
                    tar.addfile(info, io.BytesIO(data))
                else:
                    tar.add(path, arcname=name, recursive=False)
    with tarfile.open(partial) as tar:
        entries = tar.getnames()
        if any(not allowed(Path(name)) for name in entries):
            raise ValueError('Archive unexpectedly includes model weights or state')
        if 'bundle/bin/portableai' not in entries:
            raise ValueError('Archive is missing the native launcher')
        if phase == '4.5':
            if any(name.startswith('bundle/harnesses/claude/') for name in entries):
                raise ValueError('Archive unexpectedly includes proprietary Claude binaries')
            manifest = json.load(tar.extractfile('bundle/manifest.json'))
            for relative, expected in manifest.items():
                if relative.startswith('models/'):
                    continue
                value = hashlib.sha256()
                with tar.extractfile('bundle/' + relative) as stream:
                    for block in iter(lambda: stream.read(1024 * 1024), b''):
                        value.update(block)
                if value.hexdigest() != expected:
                    raise ValueError('Archive integrity mismatch: ' + relative)
    partial.rename(target)
    checksum = file_hash(target)
    (target.parent / f'{label}.sha256').write_text(checksum + '  ' + target.name + '\n')
    print(f'{target}: {target.stat().st_size:,} bytes; no weights, credentials, or runtime state')


if __name__ == '__main__':
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument('--phase', choices=("1", "2", "3", "3.1", "4", "4.5"), required=True)
    parser.add_argument('--label', required=True)
    args = parser.parse_args()
    archive_release(args.phase, args.label)
