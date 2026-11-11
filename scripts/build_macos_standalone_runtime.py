#!/usr/bin/env python3
"""Build the Apple Silicon runtime without separate llama.cpp or ggml libraries."""

import argparse
import json
from pathlib import Path
import subprocess

from build import dependencies, digest, extract_archive
from build_phase4 import ROOT, download


def build(source_archive: Path | None, output: Path) -> None:
    lock = json.loads((ROOT / 'config/artifacts.lock.json').read_text())
    source = lock['source']
    revision = source['revision']
    if source_archive is None:
        source_archive = download({**source, 'filename': f'llama-source-{revision}.tar.gz'})
    if digest(source_archive) != source['sha256']:
        raise ValueError('llama.cpp source checksum mismatch')

    source_dir = output.parent / f'llama.cpp-{revision}'
    if not source_dir.exists():
        extract_archive(source_archive, output.parent)
    flags = [
        '-DCMAKE_BUILD_TYPE=Release',
        '-DCMAKE_OSX_ARCHITECTURES=arm64',
        '-DCMAKE_OSX_DEPLOYMENT_TARGET=13.3',
        '-DBUILD_SHARED_LIBS=OFF',
        '-DGGML_BACKEND_DL=OFF',
        '-DGGML_NATIVE=OFF',
        '-DGGML_METAL=ON',
        '-DGGML_METAL_EMBED_LIBRARY=ON',
        '-DGGML_OPENMP=OFF',
        '-DGGML_RPC=OFF',
        '-DLLAMA_BUILD_TESTS=OFF',
        '-DLLAMA_BUILD_EXAMPLES=OFF',
        '-DLLAMA_BUILD_UI=OFF',
        '-DLLAMA_USE_PREBUILT_UI=OFF',
        '-DLLAMA_OPENSSL=OFF',
        '-DLLAMA_BUILD_TOOLS=ON',
        '-DLLAMA_BUILD_SERVER=ON',
        '-DGIT_EXE=',
        '-DLLAMA_BUILD_NUMBER=' + lock['runtime']['build'].removeprefix('b'),
        '-DLLAMA_BUILD_COMMIT=' + revision[:9],
    ]
    subprocess.run(['cmake', '-G', 'Ninja', '-S', str(source_dir), '-B', str(output), *flags], check=True)
    subprocess.run(['cmake', '--build', str(output), '--target', 'llama-server', 'llama-completion', '--parallel', '6'], check=True)
    for name in ('llama-server', 'llama-completion'):
        binary = output / 'bin' / name
        libraries = dependencies(binary)
        if not libraries or any(not library.startswith(('/System/Library/', '/usr/lib/')) for library in libraries):
            raise ValueError(f'{name} depends on libraries outside macOS')


if __name__ == '__main__':
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument('--source-archive', type=Path, help='Reuse a previously downloaded, pinned source archive')
    parser.add_argument('--output', type=Path, default=ROOT / '.build/mac-single-runtime/arm64')
    args = parser.parse_args()
    build(args.source_archive, args.output.resolve())
