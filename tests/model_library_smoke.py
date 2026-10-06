#!/usr/bin/env python3
"""Offline generation smoke tests for the compact library on an Apple Silicon Mac.

This checks real format/runtime compatibility, not coding-agent quality or the
memory capacity of an untested machine. Reports include the actual host RAM.
"""
import argparse
import json
import os
from pathlib import Path
import re
import signal
import subprocess
import time

ROOT = Path(__file__).resolve().parents[1]


def run(command, out, err, seconds=480):
    with out.open("w") as stdout, err.open("w") as stderr:
        child = subprocess.Popen(command, stdout=stdout, stderr=stderr,
                                 stdin=subprocess.DEVNULL, start_new_session=True)
        try:
            return child.wait(timeout=seconds)
        except subprocess.TimeoutExpired:
            os.killpg(child.pid, signal.SIGTERM)
            try:
                child.wait(timeout=10)
            except subprocess.TimeoutExpired:
                os.killpg(child.pid, signal.SIGKILL)
                child.wait()
            raise


def main():
    p = argparse.ArgumentParser(description=__doc__)
    p.add_argument("--bundle", type=Path, required=True)
    p.add_argument("model_ids", nargs="+")
    a = p.parse_args()
    models = json.loads((ROOT / "config/model-library.lock.json").read_text())["models"]
    executable = a.bundle.resolve() / "bin/macos-arm64/reflectgo"
    evidence = ROOT / "docs/evidence/model-library"
    evidence.mkdir(parents=True, exist_ok=True)
    report_path = evidence / "smoke.json"
    report = json.loads(report_path.read_text()) if report_path.exists() else {}
    hardware = json.loads(subprocess.check_output([str(executable), "doctor"], timeout=120))
    report["hardware"] = hardware
    report["scope"] = "Real offline generation on this host only. Does not qualify 8/12/24 GiB hardware or CLI tool use. Qwen 9B is tested at 1024 context on this 16 GiB host; its recommended 24 GiB profile is not physically tested."
    report.setdefault("models", {})
    for key in a.model_ids:
        item = models[key]
        context = 1024 if key == "qwen35-9b-iq4-xs" else 8192
        out, err = evidence / (key + ".txt"), evidence / (key + ".log")
        command = ["/usr/bin/sandbox-exec", "-p", "(version 1)(allow default)(deny network*)",
                   "/usr/bin/time", "-l", str(executable), "run", "--model", item["filename"],
                   "--context", str(context), "--max-tokens", "256", "--temperature", "0",
                   "--verbose", "--prompt", "What is 2 + 2? Reply with only the answer. /no_think"]
        print(f"Testing {key}, context {context}, network denied", flush=True)
        start = time.monotonic()
        code = run(command, out, err)
        answer, log = out.read_text(), err.read_text()
        speeds = re.findall(r"eval time =.*?\(\s*[\d.]+ ms per token,\s*([\d.]+) tokens per second\)", log)
        rss = re.search(r"(\d+)\s+maximum resident set size", log)
        record = {"context": context, "exit_code": code, "network_denied": True,
                  "wall_seconds": round(time.monotonic() - start, 2),
                  "generation_tokens_per_second": float(speeds[-1]) if speeds else None,
                  "maximum_rss_bytes": int(rss.group(1)) if rss else None,
                  "response": answer.strip(), "passed": code == 0 and bool(re.search(r"\b4\b|\bfour\b", answer, re.I))}
        report["models"][key] = record
        report_path.write_text(json.dumps(report, indent=2) + "\n")
        print(json.dumps({"model": key, **record}), flush=True)
        if not record["passed"]:
            raise SystemExit(f"Smoke test failed; inspect {err} and {out}")


if __name__ == "__main__":
    main()
