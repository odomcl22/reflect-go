#!/usr/bin/env python3
"""Add the pinned Claude Code CLI directly to your own Reflect Go folder.

Requires Python 3 and internet for this one-time download. Does not run an
upstream installer, modify your shell profile, or install into your home folder.
"""
import argparse
import platform
from pathlib import Path

from prepare_harnesses import TARGETS, install


def host_platform():
    system={'Darwin':'macos','Windows':'windows','Linux':'linux'}.get(platform.system())
    arch={'arm64':'arm64','aarch64':'arm64','x86_64':'x64','amd64':'x64'}.get(platform.machine().lower())
    key=f'{system}-{arch}'
    if key not in TARGETS:
        raise ValueError('Unsupported host platform; choose a supported --platform explicitly.')
    return key


def main():
    parser=argparse.ArgumentParser(description=__doc__)
    parser.add_argument('--bundle',type=Path,default=Path(__file__).resolve().parents[1])
    parser.add_argument('--platform',choices=list(TARGETS))
    args=parser.parse_args()
    bundle=args.bundle.resolve(strict=True)
    if not (bundle/'harnesses/installed.json').is_file():
        parser.error('Choose an extracted Reflect Go folder containing harnesses/installed.json.')
    selected=args.platform or host_platform()
    lock=bundle/'config/claude-cli.lock.json'
    if not lock.is_file():
        parser.error('The pinned Claude CLI download list is missing from this package.')
    install(bundle,[selected],clients=['claude'],lock_path=lock,cache_dir=bundle/'state/downloads')
    print(f'Claude Code CLI is ready for {selected}. Reopen Reflect Go to use it.')


if __name__=='__main__':
    try:
        main()
    except (OSError,ValueError,RuntimeError) as error:
        raise SystemExit(str(error))
