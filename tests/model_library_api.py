#!/usr/bin/env python3
"""Real local API smoke tests for the downloaded compact model library."""
import argparse
import json
import os
from pathlib import Path
import signal
import socket
import subprocess
import time
import urllib.error
import urllib.request

ROOT = Path(__file__).resolve().parents[1]


def stop(child):
    if child.poll() is not None:
        return
    os.killpg(child.pid, signal.SIGTERM)
    try:
        child.wait(timeout=20)
    except subprocess.TimeoutExpired:
        os.killpg(child.pid, signal.SIGKILL)
        child.wait()


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--bundle", type=Path, required=True)
    parser.add_argument("model_ids", nargs="+")
    args = parser.parse_args()
    bundle = args.bundle.resolve()
    binary = bundle / "bin/macos-arm64/reflectgo"
    models = json.loads((ROOT / "config/model-library.lock.json").read_text())["models"]
    evidence = ROOT / "docs/evidence/model-library"
    evidence.mkdir(parents=True, exist_ok=True)
    report_path = evidence / "api-smoke.json"
    report = json.loads(report_path.read_text()) if report_path.exists() else {"models": {}}
    report["hardware"] = json.loads(subprocess.check_output([str(binary), "doctor"], timeout=120))
    report["scope"] = "Real localhost API generation with the bundled runtime's --offline flag. Only this host tested; 8/12/24 GiB device tests and coding harness reliability remain unqualified."
    for key in args.model_ids:
        context = 1024 if key == "qwen35-9b-iq4-xs" else 8192
        with socket.socket() as sock:
            sock.bind(("127.0.0.1", 0))
            port = sock.getsockname()[1]
        base = f"http://127.0.0.1:{port}"
        start = time.monotonic()
        print(f"Starting {key}, context {context}, local API", flush=True)
        with (evidence / (key + "-api.log")).open("w") as log:
            child = subprocess.Popen([str(binary), "serve", "--model", models[key]["filename"],
                                      "--context", str(context), "--port", str(port), "--temperature", "0"],
                                     stdout=log, stderr=log, stdin=subprocess.DEVNULL, start_new_session=True)
            try:
                deadline = time.monotonic() + 480
                while time.monotonic() < deadline:
                    if child.poll() is not None:
                        raise RuntimeError(f"Provider exited with {child.returncode}; inspect {key}-api.log")
                    try:
                        with urllib.request.urlopen(base + "/health", timeout=2) as reply:
                            if reply.status == 200:
                                break
                    except (OSError, urllib.error.URLError):
                        pass
                    time.sleep(.5)
                else:
                    raise TimeoutError("Provider did not become ready")
                ready_seconds = round(time.monotonic() - start, 2)
                token = (bundle / "state/provider-key").read_text().strip()
                payload = {"model": "reflect-go-local", "messages": [{"role": "user", "content": "What is 2 + 2? Reply with only the number."}],
                           "temperature": 0, "max_tokens": 128, "chat_template_kwargs": {"enable_thinking": False}}
                request = urllib.request.Request(base + "/v1/chat/completions", data=json.dumps(payload).encode(),
                                                 headers={"Content-Type": "application/json", "Authorization": "Bearer " + token})
                with urllib.request.urlopen(request, timeout=180) as reply:
                    response = json.load(reply)
                answer = response["choices"][0]["message"]["content"]
                record = {"context": context, "ready_seconds": ready_seconds,
                          "wall_seconds": round(time.monotonic() - start, 2),
                          "response": answer, "usage": response.get("usage"), "timings": response.get("timings"),
                          "passed": answer.strip().strip(".* ") == "4"}
                report["models"][key] = record
                report_path.write_text(json.dumps(report, indent=2) + "\n")
                print(json.dumps({"model": key, **record}), flush=True)
                if not record["passed"]:
                    raise RuntimeError("Model did not give the expected simple response")
            finally:
                stop(child)


if __name__ == "__main__":
    main()
