#!/usr/bin/env python3
"""One-time migration of the user's original PortableAI USB into phase folders."""
import argparse
import importlib.util
import json
from pathlib import Path
import shutil
import subprocess
import tarfile

ROOT = Path(__file__).resolve().parents[1]
spec = importlib.util.spec_from_file_location("build", ROOT / "scripts/build.py")
build = importlib.util.module_from_spec(spec)
spec.loader.exec_module(build)


def migrate(volume):
    source = ROOT / "dist/Reflect Go"
    old = volume / "PortableAI"
    for name in ("Models", "Phase 1", "Phase 2"):
        if (volume / name).exists():
            raise SystemExit(f"Refusing existing destination: {volume / name}")
    if not old.is_dir() or old.is_symlink():
        raise SystemExit("Original PortableAI directory is missing or is a symlink")
    active = subprocess.run(["/usr/sbin/lsof", "+D", str(old)], text=True, capture_output=True)
    if active.stdout.strip() or active.returncode not in (0, 1):
        raise SystemExit("Original folder is in use or its process check failed")
    lock = json.loads((ROOT / "config/artifacts.lock.json").read_text())
    original_model = old / "models" / lock["model"]["name"]
    if build.digest(original_model) != lock["model"]["sha256"]:
        raise SystemExit("Original model differs from its pinned checksum; preserved without changes")
    config = json.loads((old / "config/portableai.json").read_text())
    # Preserve the exact USB software/config before updating its working launcher.
    releases = ROOT / "releases"
    releases.mkdir(exist_ok=True)
    backup = releases / "phase-1-usb-before-migration.tar.gz"
    if backup.exists():
        raise SystemExit("Migration backup already exists; refusing to overwrite it")
    with tarfile.open(backup, "w:gz") as tar:
        for path in sorted(old.rglob("*")):
            rel = path.relative_to(old)
            if rel.parts[0] not in ("models", "state") and not path.name.startswith("._"):
                tar.add(path, arcname=str(Path("PortableAI") / rel), recursive=False)
    (releases / "phase-1-usb-before-migration.sha256").write_text(build.digest(backup) + "  " + backup.name + "\n")
    models = volume / "Models"
    models.mkdir()
    agent = lock["agent_model"]
    partial = models / (agent["name"] + ".part")
    print("Copying the 4B model once to the USB shared library...", flush=True)
    shutil.copy2(source / "Models" / agent["name"], partial)
    if build.digest(partial) != agent["sha256"]:
        raise SystemExit("Copied agent model failed verification; original Phase 1 is untouched")
    partial.rename(models / agent["name"])
    skip = shutil.ignore_patterns("state", "models", ".DS_Store", "._*")
    shutil.copytree(source / "Phase 2", volume / "Phase 2", ignore=skip)
    phase1 = volume / "Phase 1"
    old.rename(phase1)
    # This is a same-volume move: there is still only one 1.5B weight file.
    (phase1 / "models" / original_model.name).rename(models / original_model.name)
    shutil.copytree(source / "Phase 1", phase1, dirs_exist_ok=True, ignore=skip)
    config["models_directory"] = "../Models"
    (phase1 / "config/portableai.json").write_text(json.dumps(config, indent=2) + "\n")
    shutil.copy2(ROOT / "README.md", volume / "README.md")
    print("USB organized. Phase 1 state preserved; both weights stored once in Models.", flush=True)


if __name__ == "__main__":
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("volume", type=Path)
    args = parser.parse_args()
    migrate(args.volume.resolve())
