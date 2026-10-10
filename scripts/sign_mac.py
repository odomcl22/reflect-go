#!/usr/bin/env python3
"""Developer ID signing and notarization for a Reflect Go phase folder.

Works only on a fresh staging copy: the source build (dist/ or a USB drive) is
never modified. Every Mach-O file Reflect Go builds or bundles is re-signed with
the hardened runtime and a secure timestamp. Vendor client packs (Codex, Claude
Code, OpenCode) keep their publishers' signatures and are only checked. Signing
changes file bytes, so the copy's manifest is updated to match.

Example:
  python3 scripts/sign_mac.py --source 'dist/Reflect Go/Phase 4.5' \
      --staging '.build/signing/Reflect Go/Phase 4.5' --exclude-client-packs \
      --identity 'Developer ID Application: YOUR NAME (TEAM ID)' \
      --notary-profile reflect-go-notary --dmg .build/signing/ReflectGo-mac.dmg
"""
import argparse
import hashlib
import json
from pathlib import Path
import shutil
import subprocess
import time

ROOT = Path(__file__).resolve().parents[1]
MACHO_MAGIC = {b"\xcf\xfa\xed\xfe", b"\xce\xfa\xed\xfe", b"\xca\xfe\xba\xbe", b"\xbe\xba\xfe\xca"}
VENDOR_PACKS = ("codex", "claude", "opencode")


def digest(path):
    h = hashlib.sha256()
    with path.open("rb") as f:
        for block in iter(lambda: f.read(1 << 20), b""):
            h.update(block)
    return h.hexdigest()


def is_macho(path):
    if path.is_symlink() or not path.is_file():
        return False
    with path.open("rb") as f:
        return f.read(4) in MACHO_MAGIC


def vendor_pack(relative):
    parts = relative.parts
    return len(parts) > 1 and parts[0] == "harnesses" and parts[1] in VENDOR_PACKS


def stage(source, staging, exclude_client_packs):
    resolved = source.resolve()
    if staging.exists():
        raise SystemExit(f"Staging folder exists: {staging}. Choose a fresh path.")
    if staging.resolve().is_relative_to(resolved):
        raise SystemExit("Staging folder must be outside the source folder.")
    if resolved.parent.name == "Reflect Go" and staging.resolve().parent == resolved.parent:
        raise SystemExit("Staging folder must not be a sibling inside the source collection.")

    def ignore(folder, names):
        relative = Path(folder).resolve().relative_to(resolved)
        skipped = {n for n in names if n in (".DS_Store",) or n.startswith("._")}
        if relative == Path("."):
            skipped.add("state")
        if exclude_client_packs and relative == Path("harnesses"):
            skipped.update(n for n in names if n in VENDOR_PACKS)
        return skipped

    shutil.copytree(resolved, staging, symlinks=True, ignore=ignore)
    manifest = staging / "manifest.json"
    hashes = json.loads(manifest.read_text())
    if exclude_client_packs:
        hashes = {p: h for p, h in hashes.items() if not vendor_pack(Path(p))}
    manifest.write_text(json.dumps(hashes, indent=2) + "\n")
    print(f"Staged {staging}", flush=True)


def codesign(identity, path):
    # The keychain and Apple's timestamp service occasionally fail transiently
    # (errSecInternalComponent); a short retry avoids restarting a long run.
    for attempt in range(3):
        result = subprocess.run(["/usr/bin/codesign", "--force", "--options", "runtime", "--timestamp",
                                 "--sign", identity, str(path)], capture_output=True, text=True)
        if result.returncode == 0:
            return
        print(f"Retrying {path.name}: {result.stderr.strip()}", flush=True)
        time.sleep(2 * (attempt + 1))
    raise SystemExit(f"Could not sign {path}: {result.stderr.strip()}")


def sign(bundle, identity):
    app = bundle / "Reflect Go.app"
    targets, vendors = [], []
    for path in sorted(bundle.rglob("*")):
        relative = path.relative_to(bundle)
        if relative.parts[0] == "state" or not is_macho(path):
            continue
        if vendor_pack(relative):
            vendors.append(path)
        elif not path.is_relative_to(app):
            targets.append(path)
    # Deepest paths first so libraries are signed before anything that loads them.
    for path in sorted(targets, key=lambda p: len(p.parts), reverse=True):
        codesign(identity, path)
    if app.exists():
        codesign(identity, app / "Contents/MacOS/engine-host")
        codesign(identity, app)
        targets.append(app)
    for path in targets:
        subprocess.run(["/usr/bin/codesign", "--verify", "--strict", str(path)], check=True)
    for path in vendors:
        details = subprocess.run(["/usr/bin/codesign", "-dv", str(path)], capture_output=True, text=True).stderr
        if "TeamIdentifier=not set" in details or "flags=0x10000(runtime)" not in details:
            raise SystemExit(f"Vendor binary lacks a hardened Developer ID signature: {path}")
    print(f"Signed {len(targets)} items; checked {len(vendors)} vendor binaries", flush=True)
    return targets


def update_manifest(bundle):
    manifest = bundle / "manifest.json"
    hashes = json.loads(manifest.read_text())
    # Signing portable helpers changes their bytes as well as the bundle hash.
    helper_index = bundle / "harnesses/helpers.json"
    if helper_index.exists():
        helpers = json.loads(helper_index.read_text())
        helpers = {name: digest(bundle/name) for name in helpers if (bundle/name).is_file()}
        helper_index.write_text(json.dumps(helpers, indent=2) + "\n")
    for path in bundle.rglob("*"):
        relative = path.relative_to(bundle).as_posix()
        if path.is_file() and not path.is_symlink() and not relative.startswith(("state/", "Models/", "models/")) and relative != "manifest.json":
            hashes.setdefault(relative, "")
    changed = 0
    for relative in list(hashes):
        path = bundle / relative
        if not relative.startswith("models/") and path.is_file():
            value = digest(path)
            if value != hashes[relative]:
                hashes[relative] = value
                changed += 1
    manifest.write_text(json.dumps(hashes, indent=2) + "\n")
    print(f"Manifest updated for {changed} re-signed files", flush=True)


def package(bundle, dmg, identity, notary):
    if dmg.exists():
        raise SystemExit(f"DMG exists: {dmg}")
    subprocess.run(["/usr/bin/hdiutil", "create", "-volname", "Reflect Go Installer", "-srcfolder", str(bundle),
                    "-format", "UDZO", "-fs", "HFS+", str(dmg)], check=True)
    subprocess.run(["/usr/bin/codesign", "--force", "--timestamp", "--sign", identity, str(dmg)], check=True)
    if notary:
        subprocess.run(["/usr/bin/xcrun", "notarytool", "submit", str(dmg), *notary, "--wait"], check=True)
        subprocess.run(["/usr/bin/xcrun", "stapler", "staple", str(dmg)], check=True)
        subprocess.run(["/usr/sbin/spctl", "--assess", "--type", "open", "--context",
                        "context:primary-signature", "-vv", str(dmg)], check=True)
    print(f"Packaged {dmg}", flush=True)


def main():
    p = argparse.ArgumentParser(description=__doc__, formatter_class=argparse.RawDescriptionHelpFormatter)
    p.add_argument("--source", type=Path, required=True)
    p.add_argument("--staging", type=Path, required=True)
    p.add_argument("--identity", required=True)
    p.add_argument("--exclude-client-packs", action="store_true",
                   help="Leave the vendor Codex/Claude/OpenCode packs out of the signed copy")
    p.add_argument("--dmg", type=Path, help="Also build and sign a disk image")
    p.add_argument("--notary-profile", help="notarytool keychain profile; notarizes and staples the DMG")
    p.add_argument("--notary-key", type=Path,
                   help="App Store Connect API key file (.p8); use instead of a keychain profile")
    p.add_argument("--notary-key-id")
    p.add_argument("--notary-issuer")
    a = p.parse_args()
    notary = None
    if a.notary_profile:
        notary = ["--keychain-profile", a.notary_profile]
    elif a.notary_key:
        if not (a.notary_key_id and a.notary_issuer):
            p.error("--notary-key requires --notary-key-id and --notary-issuer")
        notary = ["--key", str(a.notary_key.expanduser()), "--key-id", a.notary_key_id,
                  "--issuer", a.notary_issuer]
    if notary and not a.dmg:
        p.error("notarization requires --dmg")
    stage(a.source, a.staging, a.exclude_client_packs)
    sign(a.staging, a.identity)
    update_manifest(a.staging)
    if a.dmg:
        package(a.staging, a.dmg, a.identity, notary)


if __name__ == "__main__":
    main()
