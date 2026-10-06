#!/usr/bin/env python3
"""Download selected pinned GGUFs directly to a collection's shared Models folder.

Nothing is downloaded by default. Partial files are hidden from the model picker;
only a complete SHA-256 verified file is renamed to .gguf. No host download cache.
"""
import argparse
import hashlib
import json
import os
from pathlib import Path
import re
import shutil
import time
import urllib.error
import urllib.request

ROOT = Path(__file__).resolve().parents[1]
LOCK = ROOT / "config/model-library.lock.json"
CHUNK = 4 * 1024 * 1024
WINDOW = 64 * 1024 * 1024
RESERVE = 1024 ** 3


def digest(path):
    h = hashlib.sha256()
    with path.open("rb") as src:
        for block in iter(lambda: src.read(CHUNK), b""):
            h.update(block)
    return h.hexdigest()


def paths(folder, item):
    name = item["filename"]
    if Path(name).name != name or not name.endswith(".gguf") or "\\" in name:
        raise ValueError("Invalid filename in model lock")
    target = folder / name
    part = folder / ("." + name + ".part")
    for path in (target, part):
        if path.is_symlink() or (path.exists() and not path.is_file()):
            raise ValueError(f"Refusing non-regular destination: {path}")
    return target, part


def remaining(folder, item):
    target, part = paths(folder, item)
    if target.exists():
        if target.stat().st_size != item["size_bytes"]:
            raise ValueError(f"Existing file has unexpected size: {target}")
        return 0
    offset = part.stat().st_size if part.exists() else 0
    if offset > item["size_bytes"]:
        raise ValueError(f"Oversized partial file: {part}")
    return item["size_bytes"] - offset


def transfer(item, part):
    total = item["size_bytes"]
    offset = part.stat().st_size if part.exists() else 0
    if offset == total:
        return
    started = last_report = time.monotonic()
    initial = offset
    with part.open("ab") as out:
        while offset < total:
            end = min(offset + WINDOW, total) - 1
            request = urllib.request.Request(item["url"], headers={
                "Range": f"bytes={offset}-{end}", "Accept-Encoding": "identity",
                "User-Agent": "ReflectGo-model-download/1",
            })
            with urllib.request.urlopen(request, timeout=90) as response:
                if response.status == 206:
                    match = re.fullmatch(r"bytes (\d+)-(\d+)/(\d+)", response.headers.get("Content-Range", ""))
                    if not match or tuple(map(int, match.groups())) != (offset, end, total):
                        raise ValueError("Server returned an unexpected byte range")
                elif response.status != 200 or offset or end != total - 1:
                    # Never append a server's full response to a partial download.
                    raise ValueError("Server did not honor the requested resume range")
                expected = end - offset + 1
                content_length = response.headers.get("Content-Length")
                if content_length is not None and int(content_length) != expected:
                    raise ValueError("Download size differs from pinned file size")
                # Close the network response before slow USB writes, so a drive
                # stall cannot idle out a multi-gigabyte HTTP response. RAM is
                # bounded to one window and nothing is cached on the host disk.
                data = bytearray()
                while len(data) <= expected:
                    block = response.read(min(CHUNK, expected - len(data) + 1))
                    if not block:
                        break
                    data.extend(block)
                if len(data) > expected:
                    raise ValueError("Download exceeds pinned byte range")
            if shutil.disk_usage(part.parent).free < len(data) + RESERVE:
                raise OSError("Stopped to preserve 1 GiB of free space")
            out.write(data)
            offset += len(data)
            if len(data) != expected:
                raise ConnectionError(f"Incomplete download: {offset} of {total} bytes; partial retained")
            now = time.monotonic()
            if now - last_report >= 20:
                rate = (offset - initial) / max(now - started, 0.1) / 1e6
                print(f'{item["filename"]}: {offset / total:.1%} ({offset / 1e9:.2f}/{total / 1e9:.2f} GB), {rate:.1f} MB/s', flush=True)
                last_report = now
        out.flush()
        os.fsync(out.fileno())


def download(folder, item):
    target, part = paths(folder, item)
    if target.exists():
        print(f"Checking existing {target.name}", flush=True)
        if target.stat().st_size != item["size_bytes"] or digest(target) != item["sha256"]:
            raise ValueError(f"Refusing to overwrite unverified existing file: {target}")
        return
    if remaining(folder, item) + RESERVE > shutil.disk_usage(folder).free:
        raise OSError(f"Insufficient space for {target.name}, including 1 GiB reserve")
    print(f"Downloading {target.name} directly to {folder}", flush=True)
    for attempt in range(5):
        try:
            transfer(item, part)
            break
        except (urllib.error.URLError, TimeoutError, ConnectionError) as error:
            if attempt == 4:
                raise
            print(f"Connection interrupted; resuming retained partial: {error}", flush=True)
            time.sleep(2)
    print(f"Verifying SHA-256 from disk: {target.name}", flush=True)
    if part.stat().st_size != item["size_bytes"] or digest(part) != item["sha256"]:
        raise ValueError(f"Checksum mismatch; untrusted file retained as {part.name}, not published")
    if target.exists():
        raise FileExistsError(f"Destination appeared during download: {target}")
    part.rename(target)
    print(f"Verified and ready: {target.name}", flush=True)


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--models-dir", type=Path, required=True)
    parser.add_argument("--dry-run", action="store_true")
    parser.add_argument("model_ids", nargs="+")
    args = parser.parse_args()
    models = json.loads(LOCK.read_text())["models"]
    ids = list(dict.fromkeys(args.model_ids))
    unknown = set(ids) - models.keys()
    if unknown:
        parser.error("Unknown model IDs: " + ", ".join(sorted(unknown)))
    folder = args.models_dir.resolve(strict=True)
    if not folder.is_dir():
        parser.error("Models directory must already exist")
    required = sum(remaining(folder, models[key]) for key in ids)
    free = shutil.disk_usage(folder).free
    print(f"Selected: {required / 1e9:.2f} GB still to download; {free / 1e9:.2f} GB free; 1 GiB reserved", flush=True)
    if required + RESERVE > free:
        parser.error("Selected models do not fit; choose fewer models or a larger drive")
    if args.dry_run:
        return
    # Prevent two copies from consuming the same capacity or touching one partial.
    guard = folder / ".reflectgo-download.lock"
    with guard.open("x") as out:
        out.write(str(os.getpid()) + "\n")
    try:
        for key in ids:
            download(folder, models[key])
    finally:
        guard.unlink()


if __name__ == "__main__":
    try:
        main()
    except (OSError, ValueError) as error:
        raise SystemExit(str(error))
