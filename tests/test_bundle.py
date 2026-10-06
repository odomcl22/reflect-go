#!/usr/bin/env python3
"""Operational failure tests against the actual compiled launcher (no model generation)."""
import importlib.util
import io
import json
import os
from pathlib import Path
import shutil
import struct
import subprocess
import sys
import tarfile
import tempfile
import unittest

BUNDLE = Path(sys.argv.pop(1) if len(sys.argv) > 1 else "dist/PortableAI").resolve()
ROOT = Path(__file__).resolve().parents[1]
spec = importlib.util.spec_from_file_location("build", ROOT / "scripts/build.py")
build = importlib.util.module_from_spec(spec)
spec.loader.exec_module(build)


class BundleTests(unittest.TestCase):
    def setUp(self):
        self.temp = tempfile.TemporaryDirectory(prefix="PortableAI tests ü ")
        self.root = Path(self.temp.name) / "Moved folder"
        for relative in ("bin", "models", "config"):
            (self.root / relative).mkdir(parents=True, exist_ok=True)
        shutil.copy2(BUNDLE / "bin/portableai", self.root / "bin/portableai")
        shutil.copy2(BUNDLE / "config/portableai.json", self.root / "config/portableai.json")
        settings = json.loads((self.root / "config/portableai.json").read_text())
        settings["models_directory"] = "models"
        (self.root / "config/portableai.json").write_text(json.dumps(settings))
        (self.root / "manifest.json").write_text("{}")
        self.env = {"PATH": "/usr/bin:/bin:/usr/sbin:/sbin", "HOME": self.temp.name, "LANG": "en_US.UTF-8"}

    def tearDown(self):
        self.temp.cleanup()

    def run_cli(self, *args):
        return subprocess.run([str(self.root / "bin/portableai"), *args], text=True,
                              capture_output=True, timeout=20, env=self.env, cwd="/")

    def fails(self, text, *args):
        result = self.run_cli(*args)
        self.assertNotEqual(result.returncode, 0, result.stdout + result.stderr)
        self.assertIn(text, result.stderr)

    def header(self, name="test.gguf", size=2048):
        path = self.root / "models" / name
        with path.open("wb") as f:
            f.write(b"GGUF" + struct.pack("<IQQ", 3, 1, 1))
            f.truncate(size)
        return path

    def test_relocated_native_launcher_with_system_path(self):
        result = self.run_cli("doctor")
        self.assertEqual(result.returncode, 0, result.stderr)
        hardware = json.loads(result.stdout)
        self.assertGreater(hardware["physicalMemoryBytes"], 0)
        self.assertEqual(hardware["architecture"], "arm64")
        self.assertLessEqual(hardware["screeningBudgetBytes"], hardware["physicalMemoryBytes"] / 2)

    def test_missing_model(self):
        self.fails("No models found", "run", "--prompt", "hello")

    def test_bad_magic(self):
        (self.root / "models/bad.gguf").write_bytes(b"not a model" * 200)
        self.fails("not a GGUF", "run", "--model", "bad.gguf", "--prompt", "hello")

    def test_partial_download_header(self):
        (self.root / "models/bad.gguf").write_bytes(b"GGUF")
        self.fails("incomplete header", "run", "--model", "bad.gguf", "--prompt", "hello")

    def test_external_model_symlink(self):
        outside = Path(self.temp.name) / "outside.gguf"
        outside.write_bytes(b"GGUF")
        (self.root / "models/outside.gguf").symlink_to(outside)
        self.fails("symlink points outside", "run", "--model", "outside.gguf", "--prompt", "hello")

    def test_split_model(self):
        self.header("test-00001-of-00002.gguf")
        self.fails("split GGUF", "run", "--model", "test-00001-of-00002.gguf", "--prompt", "hello")

    def test_oversized_model_refused_before_runtime(self):
        hardware = json.loads(self.run_cli("doctor").stdout)
        self.header(size=hardware["physicalMemoryBytes"])
        self.fails("exceeds the conservative memory budget", "run", "--model", "test.gguf", "--prompt", "hello")

    def test_missing_runtime(self):
        self.header()
        self.fails("Bundled runtime is missing", "run", "--prompt", "hello")

    def test_invalid_configuration_and_context(self):
        (self.root / "config/portableai.json").write_text("not json")
        self.fails("Cannot read config", "models")
        shutil.copy2(BUNDLE / "config/portableai.json", self.root / "config/portableai.json")
        self.fails("context must be", "models", "--context", "0")
        self.fails("max_tokens must be", "models", "--max-tokens", "4096")

    def test_unknown_argument_is_not_passed_to_runtime(self):
        self.fails("Unknown option", "run", "--prompt", "hello", "--rpc", "example.invalid")

    def test_shared_models_and_path_escape(self):
        shared = self.root.parent / "Models"
        shared.mkdir()
        self.header().rename(shared / "test.gguf")
        config = self.root / "config/portableai.json"
        settings = json.loads(config.read_text())
        settings["models_directory"] = "../Models"
        config.write_text(json.dumps(settings))
        result = self.run_cli("models")
        self.assertEqual(result.returncode, 0, result.stderr)
        self.assertIn("test.gguf", result.stderr)
        settings["models_directory"] = "../../elsewhere"
        config.write_text(json.dumps(settings))
        self.fails("models_directory must be", "models")

    def test_inventory_empty_and_context_dependent_models(self):
        empty = json.loads(self.run_cli("inventory").stdout)
        self.assertEqual(empty["models"], [])
        self.header()
        low = json.loads(self.run_cli("inventory", "--context", "2048").stdout)
        high = json.loads(self.run_cli("inventory", "--context", "8192").stdout)
        self.assertEqual(low["models"][0]["name"], "test.gguf")
        self.assertGreater(high["models"][0]["estimatedBytes"], low["models"][0]["estimatedBytes"])
        self.assertEqual(high["context"], 8192)
        (self.root / "models/damaged.gguf").write_bytes(b"damaged")
        snapshot = json.loads(self.run_cli("inventory").stdout)
        damaged = next(model for model in snapshot["models"] if model["name"] == "damaged.gguf")
        self.assertFalse(damaged["eligible"])
        self.assertIn("GGUF", damaged["issue"])

    def test_invalid_performance_overrides(self):
        for value in ("nan", "inf", "-1", "3", "warm"):
            self.fails("--temperature requires", "inventory", "--temperature", value)
        for value in ("0", "257", "all"):
            self.fails("--threads requires", "inventory", "--threads", value)
        hardware = json.loads(self.run_cli("doctor").stdout)
        self.fails("exceeds this computer", "inventory", "--threads", str(hardware["cpuThreads"] + 1))

    def test_exfat_metadata_is_not_a_model(self):
        self.header()
        (self.root / "models/._test.gguf").write_bytes(b"AppleDouble metadata")
        result = json.loads(self.run_cli("inventory").stdout)
        self.assertEqual([model["name"] for model in result["models"]], ["test.gguf"])

    def test_invalid_provider_ports(self):
        for value in ("0", "80", "65536", "localhost"):
            self.fails("--port requires", "serve", "--port", value)
        self.fails("Provider clients set their own output budget", "serve", "--max-tokens", "32")

    def test_runtime_does_not_start_for_missing_prompt(self):
        self.fails("run requires", "run")

    def test_tampered_packaged_model(self):
        self.header()
        (self.root / "manifest.json").write_text(json.dumps({"models/test.gguf": "0" * 64}))
        self.fails("Model checksum mismatch", "run", "--prompt", "hello")
        self.fails("Integrity check failed", "verify")

    def test_manifest_cannot_escape(self):
        (self.root / "manifest.json").write_text(json.dumps({"../secret": "0" * 64}))
        self.fails("Integrity check failed", "verify")

    def test_damaged_cached_download_refused(self):
        cache = self.root / "cache"
        cache.mkdir()
        (cache / "bad").write_bytes(b"damaged")
        old = build.CACHE
        build.CACHE = cache
        try:
            with self.assertRaisesRegex(ValueError, "checksum mismatch"):
                build.download("https://example.invalid/not-used", "bad", "0" * 64)
        finally:
            build.CACHE = old

    def test_archive_traversal_and_escaping_symlink_rejected(self):
        for name, link in [("../escape", None), ("safe/link", "../../escape")]:
            archive = self.root / "malicious.tar"
            with tarfile.open(archive, "w") as tar:
                member = tarfile.TarInfo(name)
                if link:
                    member.type = tarfile.SYMTYPE
                    member.linkname = link
                    tar.addfile(member)
                else:
                    member.size = 1
                    tar.addfile(member, io.BytesIO(b"x"))
            with self.assertRaises(ValueError):
                build.extract_archive(archive, self.root / "extracted")

    def test_builder_preserves_existing_destination(self):
        with self.assertRaisesRegex(ValueError, "Destination exists"):
            build.assemble(self.root)
        self.assertTrue((self.root / "bin/portableai").is_file())


if __name__ == "__main__":
    unittest.main(verbosity=2)
