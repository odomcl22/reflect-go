#!/usr/bin/env python3
"""Assembly machine only. The produced bundle has no Python/Xcode dependency."""
import argparse
import hashlib
import json
import os
import plistlib
from pathlib import Path, PurePosixPath
import shutil
import subprocess
import tarfile
import tempfile

ROOT = Path(__file__).resolve().parents[1]
CACHE = ROOT / ".build/downloads"


def digest(path):
    h = hashlib.sha256()
    with open(path, "rb") as f:
        for chunk in iter(lambda: f.read(1024 * 1024), b""):
            h.update(chunk)
    return h.hexdigest()


def download(url, name, checksum=None):
    CACHE.mkdir(parents=True, exist_ok=True)
    path = CACHE / name
    if not path.exists():
        partial = path.with_suffix(path.suffix + ".part")
        subprocess.run(["/usr/bin/curl", "-fL", "--proto", "=https", "--proto-redir", "=https",
                        "--retry", "2", "--connect-timeout", "20", "--max-time", "1200",
                        url, "-o", str(partial)], check=True)
        if checksum and digest(partial) != checksum:
            raise ValueError(f"Download checksum mismatch: {name}; partial file retained for inspection")
        partial.replace(path)
    if checksum and digest(path) != checksum:
        raise ValueError(f"Cached artifact checksum mismatch: {path}; remove it explicitly to retry")
    return path


def extract_archive(archive, destination):
    # Archives are checksum-pinned, but still reject traversal and special files.
    with tarfile.open(archive) as tar:
        for member in tar.getmembers():
            path = PurePosixPath(member.name)
            if path.is_absolute() or ".." in path.parts:
                raise ValueError(f"Unsafe archive path: {member.name}")
            if not (member.isfile() or member.isdir() or member.issym()):
                raise ValueError(f"Unsupported archive entry: {member.name}")
            if member.issym():
                target = (destination / member.name).parent / member.linkname
                if os.path.commonpath([target.resolve(), destination.resolve()]) != str(destination.resolve()):
                    raise ValueError(f"Escaping archive symlink: {member.name}")
        # Extract directories and files ourselves; no archive link can redirect writes.
        for member in tar.getmembers():
            path = destination / member.name
            if member.isdir():
                path.mkdir(parents=True, exist_ok=True)
            elif member.isfile():
                path.parent.mkdir(parents=True, exist_ok=True)
                with tar.extractfile(member) as source, path.open("wb") as target:
                    shutil.copyfileobj(source, target)
                path.chmod(member.mode & 0o777)
        for member in tar.getmembers():
            if member.issym():
                (destination / member.name).symlink_to(member.linkname)


def dependencies(path):
    output = subprocess.check_output(["/usr/bin/otool", "-L", str(path)], text=True)
    return [line.strip().split(" (", 1)[0] for line in output.splitlines()[1:]]


def audit(path, runtime_dir):
    description = subprocess.check_output(["/usr/bin/file", str(path)], text=True)
    if "arm64" not in description or "Mach-O" not in description:
        raise ValueError(f"Not a native ARM64 binary: {path}")
    for dep in dependencies(path):
        if dep.startswith(("/usr/lib/", "/System/Library/")):
            continue
        if dep.startswith(("@rpath/", "@loader_path/")) and (runtime_dir / dep.split("/", 1)[1]).is_file():
            continue
        raise ValueError(f"Unbundled dependency in {path.name}: {dep}")
    load = subprocess.check_output(["/usr/bin/otool", "-l", str(path)], text=True)
    lines = load.splitlines()
    for i, line in enumerate(lines):
        if "LC_RPATH" in line:
            rpath = lines[i + 2].strip().split(" (", 1)[0].removeprefix("path ")
            if not rpath.startswith(("@loader_path", "@executable_path", "/usr/lib/swift")):
                raise ValueError(f"Nonportable library search path: {rpath}")
    return {"file": path.name, "dependencies": dependencies(path),
            "minimum_os": [line.strip() for line in lines if line.strip().startswith("minos ")]}


def write_manifest(bundle):
    # Configuration is intentionally editable; extra user models are not in this manifest.
    manifest = {str(p.relative_to(bundle)): digest(p) for p in sorted(bundle.rglob("*"))
                if p.is_file() and p.name != "manifest.json"
                and p.name != ".DS_Store" and not p.name.startswith("._")
                and p.relative_to(bundle).parts[0] not in ("config", "state")}
    (bundle / "manifest.json").write_text(json.dumps(manifest, indent=2) + "\n")


def assemble(output, phase=1, shared_models=None):
    stage = int(str(phase).split(".")[0])
    validation_phase = str(phase).replace(".", "_")
    if output.exists():
        raise ValueError(f"Destination exists: {output}. Choose a fresh --output; existing models/config are preserved.")
    lock = json.loads((ROOT / "config/artifacts.lock.json").read_text())
    if shared_models is not None and shared_models.resolve() != output.parent / "Models":
        raise ValueError("Shared models must be the sibling Models folder")
    selected_model = lock["agent_model"] if stage >= 2 else lock["model"]
    runtime_tar = download(lock["runtime"]["url"], "runtime.tar.gz", lock["runtime"]["sha256"])
    model = download(selected_model["url"], selected_model["name"], selected_model["sha256"])
    source_tar = download(lock["source"]["url"], "source.tar.gz", lock["source"]["sha256"])
    # Model notices are pinned to the same immutable revision; record their hashes in the bundle.
    model_base = f'https://huggingface.co/{lock["model"]["repository"]}/resolve/{lock["model"]["revision"]}'
    model_license = download(model_base + "/LICENSE", lock["model"]["revision"] + "-LICENSE")
    model_card = download(model_base + "/README.md", lock["model"]["revision"] + "-MODEL-CARD.md")
    output.parent.mkdir(parents=True, exist_ok=True)
    with tempfile.TemporaryDirectory(prefix="portableai-assembly-", dir=ROOT / ".build") as temporary:
        work = Path(temporary)
        extract_archive(runtime_tar, work / "runtime")
        extract_archive(source_tar, work / "source")
        upstream = work / "runtime" / ("llama-" + lock["runtime"]["build"])
        source = work / "source" / ("llama.cpp-" + lock["source"]["revision"])
        bundle = work / "PortableAI"
        runtime = bundle / "runtime/macos-arm64"
        for path in [runtime, bundle / "bin", bundle / "models", bundle / "config", bundle / "licenses"]:
            path.mkdir(parents=True)
        pending = ["llama-completion"] + (["llama-server"] if stage >= 2 else [])
        copied = set()
        while pending:
            name = pending.pop()
            if name in copied:
                continue
            shutil.copy2(upstream / name, runtime / name)  # Flatten internal dylib symlinks for exFAT.
            copied.add(name)
            for dep in dependencies(runtime / name):
                if dep.startswith(("@rpath/", "@loader_path/")):
                    pending.append(dep.split("/", 1)[1])
        subprocess.run(["/usr/bin/xcrun", "swiftc", "-O", "-target", "arm64-apple-macosx13.3",
                        "-module-cache-path", str(ROOT / ".build/module-cache"),
                        str(ROOT / "Sources/PortableAI.swift"), "-o", str(bundle / "bin/portableai")], check=True)
        if stage == 3:
            subprocess.run(["/usr/bin/xcrun", "clang", "-O2", "-Wall", "-Wextra", "-Werror",
                            "-target", "arm64-apple-macosx13.3", str(ROOT / "Sources/EngineHost.c"),
                            "-o", str(bundle / "bin/engine-host")], check=True)
            app = bundle / "Reflect Go.app/Contents"
            (app / "MacOS").mkdir(parents=True)
            info = {"CFBundleExecutable": "ReflectGo", "CFBundleIdentifier": "local.reflectgo.workstation",
                    "CFBundleName": "Reflect Go", "CFBundleDisplayName": "Reflect Go",
                    "CFBundlePackageType": "APPL", "CFBundleShortVersionString": "0.3.1" if str(phase) == "3.1" else "0.3.0",
                    "CFBundleVersion": "31" if str(phase) == "3.1" else "3", "LSMinimumSystemVersion": "13.3",
                    "NSHighResolutionCapable": True, "NSPrincipalClass": "NSApplication",
                    "NSRemovableVolumesUsageDescription": "Reflect Go reads models and saves engine settings on the portable drive you open it from.",
                    "NSAppTransportSecurity": {"NSAllowsLocalNetworking": True}}
            (app / "Info.plist").write_bytes(plistlib.dumps(info))
            subprocess.run(["/usr/bin/xcrun", "swiftc", "-parse-as-library", "-O", "-target", "arm64-apple-macosx13.3",
                            "-module-cache-path", str(ROOT / ".build/module-cache"),
                            str(ROOT / "Sources/Workstation.swift"), "-o", str(app / "MacOS/ReflectGo")], check=True)
            subprocess.run(["/usr/bin/codesign", "--force", "--sign", "-", str(app.parent)], check=True)
        checks = [audit(p, runtime) for p in sorted(runtime.iterdir())]
        checks.append(audit(bundle / "bin/portableai", runtime))
        if stage == 3:
            checks.append(audit(bundle / "bin/engine-host", runtime))
            checks.append(audit(bundle / "Reflect Go.app/Contents/MacOS/ReflectGo", runtime))
        (bundle / "dependency-audit.json").write_text(json.dumps(checks, indent=2) + "\n")
        for name in ("PortableAI.command", "VerifyPortability.command"):
            shutil.copy2(ROOT / name, bundle / name)
            (bundle / name).chmod(0o755)
        shutil.copy2(ROOT / "PortableAI.command", bundle / "Reflect Go.command")
        if stage == 3:
            (bundle / "Reflect Go.command").write_text('#!/bin/zsh -f\nROOT=${0:A:h}\nexec /usr/bin/open "$ROOT/Reflect Go.app"\n')
        (bundle / "Reflect Go.command").chmod(0o755)
        settings = json.loads((ROOT / "config/portableai.json").read_text())
        if shared_models is not None:
            settings["models_directory"] = "../Models"
        if stage >= 2:
            settings.update(provider_context=16384, provider_model=selected_model["name"], temperature=0.2)
            for name in ("Start Provider.command", "Connect Codex.command", "Connect Claude.command"):
                shutil.copy2(ROOT / name, bundle / name)
                (bundle / name).chmod(0o755)
            shutil.copy2(ROOT / "config/codex-local-instructions.txt", bundle / "config/codex-local-instructions.txt")
            shutil.copy2(ROOT / "config/claude-local-instructions.txt", bundle / "config/claude-local-instructions.txt")
            shutil.copy2(ROOT / "scripts/client-common.zsh", bundle / "bin/client-common.zsh")
            shutil.copy2(ROOT / f"docs/PHASE_{validation_phase}_VALIDATION.md", bundle / "VALIDATION.md")
        (bundle / "config/portableai.json").write_text(json.dumps(settings, indent=2) + "\n")
        shutil.copy2(ROOT / "config/artifacts.lock.json", bundle / "artifacts.lock.json")
        target_models = shared_models or bundle / "models"
        target_models.mkdir(parents=True, exist_ok=True)
        target_model = target_models / selected_model["name"]
        if target_model.exists():
            if digest(target_model) != selected_model["sha256"]:
                raise ValueError(f"Existing shared model has unexpected checksum: {target_model}")
        else:
            partial = target_model.with_suffix(".gguf.part")
            if partial.exists():
                raise ValueError(f"Incomplete model copy exists; inspect it first: {partial}")
            shutil.copy2(model, partial)
            if digest(partial) != selected_model["sha256"]:
                raise ValueError("Model copy failed integrity verification")
            partial.replace(target_model)
        shutil.copy2(ROOT / "README.md", bundle / "README.md")
        shutil.copy2(model_license, bundle / "licenses/Qwen-LICENSE")
        shutil.copy2(model_card, bundle / "licenses/Qwen-MODEL-CARD.md")
        if stage >= 2:
            base = f'https://huggingface.co/{selected_model["original_repository"]}/resolve/{selected_model["original_revision"]}'
            for upstream_name, target in [("LICENSE", "Qwen3-4B-LICENSE"), ("README.md", "Qwen3-4B-MODEL-CARD.md")]:
                notice = download(base + "/" + upstream_name, selected_model["original_revision"] + "-" + upstream_name)
                shutil.copy2(notice, bundle / "licenses" / target)
            quant_card = download(f'https://huggingface.co/{selected_model["repository"]}/resolve/{selected_model["revision"]}/README.md', selected_model["revision"] + "-MODEL-CARD.md")
            shutil.copy2(quant_card, bundle / "licenses/Qwen3-4B-GGUF-MODEL-CARD.md")
        # Preserve upstream notices and headers containing embedded third-party license text.
        for p in source.rglob("*"):
            relative = p.relative_to(source)
            if p.is_file() and (p.name.upper().startswith(("LICENSE", "COPYING", "NOTICE"))
                                or (relative.parts[0] == "vendor" and p.suffix in (".h", ".hpp", ".c"))):
                target = bundle / "licenses/llama.cpp" / relative
                target.parent.mkdir(parents=True, exist_ok=True)
                shutil.copy2(p, target)
        write_manifest(bundle)
        if shared_models is not None:
            manifest = json.loads((bundle / "manifest.json").read_text())
            manifest["models/" + selected_model["name"]] = selected_model["sha256"]
            (bundle / "manifest.json").write_text(json.dumps(manifest, indent=2) + "\n")
        shutil.move(str(bundle), str(output))
    copy_root = output.parent if shared_models else output
    print(f"Assembled {output}\nCopy the complete {copy_root} folder to the external drive. Destination hosts need no build tools.")


if __name__ == "__main__":
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--output", type=Path, default=ROOT / "dist/PortableAI")
    parser.add_argument("--phase", choices=("1", "2", "3", "3.1"), default="1")
    parser.add_argument("--shared-models", type=Path)
    args = parser.parse_args()
    try:
        assemble(args.output.resolve(), args.phase, args.shared_models.resolve() if args.shared_models else None)
    except (ValueError, OSError, subprocess.CalledProcessError) as error:
        parser.exit(1, f"Assembly failed: {error}\n")
