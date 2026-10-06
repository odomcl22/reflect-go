#!/usr/bin/env python3
"""Real inference and lifecycle acceptance checks. Run on an unrestricted test Mac.

Uses the bundled real model, macOS sandbox-exec to deny networking, and a PTY for
terminal input/signals. Evidence stays in .build/evidence; no server or credentials.
"""
import ast
import errno
import json
import os
from pathlib import Path
import pty
import re
import select
import shutil
import signal
import subprocess
import sys
import tempfile
import time

ROOT = Path(__file__).resolve().parents[1]
BUNDLE = Path(sys.argv[1] if len(sys.argv) > 1 else ROOT / "dist/PortableAI").resolve()
EVIDENCE = ROOT / ".build/evidence"
EVIDENCE.mkdir(parents=True, exist_ok=True)


def generate(label, bundle, prompt, *options, env=None, deny_network=False):
    cmd = [str(bundle / "PortableAI.command"), "run", "--verbose", "--max-tokens", "192",
           "--prompt", prompt, *options]
    if deny_network:
        cmd = ["/usr/bin/sandbox-exec", "-p", "(version 1)(allow default)(deny network*)", *cmd]
    start = time.monotonic()
    with (EVIDENCE / f"{label}.txt").open("w") as out, (EVIDENCE / f"{label}.log").open("w") as err:
        result = subprocess.run(["/usr/bin/time", "-l", *cmd], stdout=out, stderr=err,
                                timeout=180, cwd="/", env=env)
    text = (EVIDENCE / f"{label}.txt").read_text()
    log = (EVIDENCE / f"{label}.log").read_text()
    assert result.returncode == 0, f"{label} failed: {log[-2500:]}"
    assert len(text.strip()) > 10, f"{label}: empty response"
    speeds = re.findall(r"eval time =.*?\(\s*[\d.]+ ms per token,\s*([\d.]+) tokens per second\)", log)
    rss = re.search(r"(\d+)\s+maximum resident set size", log)
    print(f"PASS {label}: {time.monotonic() - start:.2f}s wall; see saved output", flush=True)
    return {"label": label, "seconds": round(time.monotonic() - start, 2),
            "generation_tokens_per_second": float(speeds[-1]) if speeds else None,
            "maximum_rss_bytes": int(rss.group(1)) if rss else None,
            "response": text}, log


def terminal_case(label, mode):
    pid, master = pty.fork()
    if pid == 0:
        os.chdir("/")
        os.execve(str(BUNDLE / "PortableAI.command"), [str(BUNDLE / "PortableAI.command"), "chat"],
                  {"PATH": "/usr/bin:/bin:/usr/sbin:/sbin", "HOME": "/nonexistent", "TERM": "xterm-256color"})
    captured = bytearray()
    reaped = False

    def read_until(marker, timeout=120):
        start_at = len(captured)
        end = time.monotonic() + timeout
        while time.monotonic() < end:
            if select.select([master], [], [], 0.1)[0]:
                try:
                    chunk = os.read(master, 65536)
                except OSError as error:
                    if error.errno == errno.EIO:
                        break
                    raise
                if not chunk:
                    break
                captured.extend(chunk)
                if marker in captured[start_at:]:
                    return
        raise AssertionError(f"{label}: never saw {marker!r}: {captured[-1500:]!r}")

    try:
        read_until(b"Choose a model")
        os.write(master, b"\n")
        read_until(b"\n> ")
        # Check the actual foreground runtime rather than all host processes.
        listeners = subprocess.run(["/usr/sbin/lsof", "-nP", "-a", "-p", str(pid), "-i"],
                                   text=True, capture_output=True)
        assert listeners.returncode == 1 and not listeners.stdout.strip(), "Unexpected network socket"
        children = subprocess.run(["/usr/bin/pgrep", "-P", str(pid)], capture_output=True)
        assert children.returncode == 1, "Unexpected background child"
        if mode == "eof":
            os.write(master, b"Reply with the word hello.\n")
            read_until(b"\n> ")
            assert b"hello" in captured.lower()
            os.write(master, b"\x04")
        elif mode == "interrupt":
            os.write(master, b"Write a long story about a voyage across the ocean.\n")
            # Wait until output begins after the echoed input, then interrupt generation.
            read_until(b"ocean.")
            time.sleep(0.3)
            os.kill(pid, signal.SIGINT)
            time.sleep(0.2)
            os.kill(pid, signal.SIGINT)
        else:
            os.kill(pid, signal.SIGHUP)
        end = time.monotonic() + 10
        status = None
        while time.monotonic() < end:
            found, status = os.waitpid(pid, os.WNOHANG)
            if found:
                reaped = True
                break
            if select.select([master], [], [], 0.1)[0]:
                try:
                    captured.extend(os.read(master, 65536))
                except OSError:
                    pass
        assert reaped, f"{label}: process survived shutdown"
        if mode == "eof":
            assert os.WIFEXITED(status) and os.WEXITSTATUS(status) == 0, status
        print(f"PASS {label}: process exited, no child or network socket", flush=True)
        return {"label": label, "wait_status": status, "network_sockets": 0, "children": 0}
    finally:
        (EVIDENCE / f"{label}.txt").write_bytes(captured)
        os.close(master)
        if not reaped:
            os.kill(pid, signal.SIGKILL)
            os.waitpid(pid, 0)


def check_generated_code(text):
    blocks = re.findall(r"```(?:python)?\s*\n(.*?)```", text, re.S)
    assert blocks, "No Python code block in generated response"
    code = blocks[0].strip()
    tree = ast.parse(code)
    # Execute only a deliberately restricted expression-only function. Model output is untrusted.
    allowed = (ast.Module, ast.FunctionDef, ast.arguments, ast.arg, ast.Return, ast.Expr,
               ast.Constant, ast.Name, ast.Load, ast.Call)
    assert all(isinstance(node, allowed) for node in ast.walk(tree)), code
    assert len(tree.body) == 1 and isinstance(tree.body[0], ast.FunctionDef)
    assert tree.body[0].name == "clamp" and not tree.body[0].decorator_list
    for node in ast.walk(tree):
        if isinstance(node, ast.Call):
            assert isinstance(node.func, ast.Name) and node.func.id in ("min", "max")
    namespace = {"__builtins__": {"min": min, "max": max}}
    exec(compile(tree, "<local-model-clamp>", "exec"), namespace)
    for values, expected in [((-5, 0, 10), 0), ((5, 0, 10), 5), ((15, 0, 10), 10),
                             ((0, 0, 10), 0), ((10, 0, 10), 10)]:
        assert namespace["clamp"](*values) == expected
    (EVIDENCE / "generated-clamp.py").write_text(code + "\n")
    return "Generated clamp function passed 5 boundary/interior cases. This is not an agent workflow."


def main():
    results = []
    summary_prompt = ("Summarize this note in one sentence, preserving both days, both times, and online borrowing availability: The library will close at 3 pm on Friday "
                      "for repairs and reopen Monday at 9 am. Online borrowing will remain available throughout.")
    summary, metal_log = generate("final-metal-summary", BUNDLE, summary_prompt)
    assert "offloaded" in metal_log and "GPU" in metal_log, "No actual GPU offload evidence"
    assert all(word in summary["response"].lower() for word in ["friday", "monday", "online", "3", "9"])
    results.append(summary)
    code, cpu_log = generate("final-cpu-code", BUNDLE,
                            "Write only a Python code block defining clamp(value, low, high). "
                            "Use one return statement with nested min and max calls to keep value "
                            "within the inclusive range low to high. No annotations, examples, or explanation.", "--cpu")
    assert not re.search(r"offloaded [1-9]\d*/\d+ layers to GPU", cpu_log), "CPU test offloaded layers"
    code["validation"] = check_generated_code(code["response"])
    results.append(code)
    with tempfile.TemporaryDirectory(prefix="PortableAI relocation ü ") as temporary:
        moved = Path(temporary) / "External Drive simulation" / "Portable AI"
        moved.parent.mkdir()
        shutil.copytree(BUNDLE, moved, ignore=shutil.ignore_patterns("state"))
        clean_home = Path(temporary) / "empty-home"
        clean_home.mkdir()
        env = {"PATH": "/usr/bin:/bin:/usr/sbin:/sbin", "HOME": str(clean_home),
               "LANG": "en_US.UTF-8", "LLAMA_ARG_MODEL_URL": "https://example.invalid/never",
               "LLAMA_ARG_RPC": "203.0.113.1:50052", "LLAMA_ARG_HF_REPO": "invalid/never"}
        relocated, _ = generate("relocated-offline", moved, summary_prompt, env=env, deny_network=True)
        assert "library" in relocated["response"].lower()
        assert not list(clean_home.iterdir()), "Original HOME was written"
        assert (moved / "state/home").is_dir()
        relocated["checks"] = ["network denied by macOS sandbox", "fresh HOME untouched", "system-only PATH",
                               "spaces and Unicode in path", "inherited remote runtime options ignored"]
        results.append(relocated)
    for label, mode in [("interactive-eof", "eof"), ("generation-interrupt", "interrupt"), ("terminal-hangup", "hangup")]:
        results.append(terminal_case(label, mode))
    (EVIDENCE / "integration-results.json").write_text(json.dumps(results, indent=2) + "\n")
    print("All real inference and lifecycle checks passed.")


if __name__ == "__main__":
    main()
