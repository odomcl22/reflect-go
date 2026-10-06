#!/usr/bin/env python3
"""Run an installed client through a packaged connector against a running provider.

Example: python3 tests/client_acceptance.py '/Volumes/Reflect Go/Phase 2' claude usb --offline
The model, not this test, must read/edit the fixture and execute its tests.
"""
import argparse
import json
from pathlib import Path
import shutil
import subprocess
import sys
import time

ROOT = Path(__file__).resolve().parents[1]
parser = argparse.ArgumentParser(description=__doc__)
parser.add_argument("bundle", type=Path)
parser.add_argument("client", choices=("codex", "claude"))
parser.add_argument("label")
parser.add_argument("--offline", action="store_true")
args = parser.parse_args()
if not args.label.replace("-", "").isalnum():
    parser.error("Use an alphanumeric label")
if args.offline and args.client != "claude":
    parser.error("The external-network-denied harness is validated with Claude Code only")
work = ROOT / (".build/phase2/acceptance-" + args.label)
if work.exists():
    parser.error("Use a fresh label; previous evidence is preserved")
shutil.copytree(ROOT / "tests/fixtures/agent-project", work)
prompt = ("Inspect math.sh and test.sh in this directory. Fix clamp in math.sh so it constrains a number "
          "to the inclusive interval low to high. Make the actual edit with your file tools, then run "
          "sh test.sh. Do not change test.sh. Report the test result.")
wrapper = args.bundle.resolve() / ("Connect Codex.command" if args.client == "codex" else "Connect Claude.command")
cmd = [str(wrapper), str(work)]
if args.client == "claude":
    cmd += ["--print", "--no-session-persistence", "--output-format", "stream-json", "--verbose",
            "--permission-mode", "dontAsk", "--allowedTools", "Read", "Edit", "Bash(sh test.sh)", "--", prompt]
else:
    cmd += [prompt]
if args.offline:
    profile = '(version 1)(allow default)(deny network-outbound)(allow network-outbound (remote ip "localhost:*"))'
    # Require an OS permission denial, rather than mistaking TLS/network errors or
    # a malformed sandbox profile for a successful isolation check.
    probe = '''import socket,errno,json
try:
    sock=socket.create_connection(("1.1.1.1",443),timeout=3)
    sock.close()
    raise SystemExit("Unexpected outbound access")
except OSError as error:
    print(json.dumps({"errno":error.errno,"error":str(error)}))
    raise SystemExit(0 if error.errno==errno.EPERM else 1)
'''
    denial = subprocess.run(["/usr/bin/sandbox-exec", "-p", profile, sys.executable, "-c", probe],
                            capture_output=True, text=True)
    assert denial.returncode == 0, denial.stdout + denial.stderr
    (ROOT / ".build/phase2/evidence" / (args.label + "-network-denial.json")).write_text(denial.stdout)
    cmd = ["/usr/bin/sandbox-exec", "-p", profile] + cmd
evidence = ROOT / ".build/phase2/evidence"
started = time.monotonic()
with (evidence / (args.label + ".txt")).open("w") as out:
    result = subprocess.run(cmd, stdout=out, stderr=subprocess.STDOUT, timeout=600)
check = subprocess.run(["/bin/sh", "test.sh"], cwd=work, text=True, capture_output=True)
unchanged = (work / "test.sh").read_bytes() == (ROOT / "tests/fixtures/agent-project/test.sh").read_bytes()
edited = (work / "math.sh").read_bytes() != (ROOT / "tests/fixtures/agent-project/math.sh").read_bytes()
record = {"client": args.client, "bundle": str(args.bundle), "external_network_denied": args.offline,
          "client_exit": result.returncode, "independent_test_exit": check.returncode,
          "tests_unchanged": unchanged, "source_edited": edited,
          "elapsed_seconds": round(time.monotonic() - started, 2), "test_output": check.stdout}
(evidence / (args.label + "-result.json")).write_text(json.dumps(record, indent=2) + "\n")
print(json.dumps(record, indent=2))
assert result.returncode == 0 and check.returncode == 0 and unchanged and edited
