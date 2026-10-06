#!/usr/bin/env python3
"""Use the installed, unmodified Codex CLI against the local prototype provider."""
import json
import os
from pathlib import Path
import shutil
import subprocess
import sys

ROOT = Path(__file__).resolve().parents[1]
label = sys.argv[1] if len(sys.argv) > 1 else "4b"
if not label.replace("-", "").isalnum():
    raise SystemExit("Use an alphanumeric test label")
work = ROOT / (".build/phase2/agent-project-" + label)
if work.exists():
    raise SystemExit("Test project exists; preserve its results and choose a fresh path.")
shutil.copytree(ROOT / "tests/fixtures/agent-project", work)
cmd = [shutil.which("codex"), "exec", "--ignore-user-config", "--ephemeral", "--skip-git-repo-check",
       "--sandbox", "workspace-write", "--json", "--color", "never", "-C", str(work)]
overrides = {
    "model": "reflect-go-local", "model_provider": "reflect_go",
    "model_providers.reflect_go.name": "Reflect Go",
    "model_providers.reflect_go.base_url": "http://127.0.0.1:8765/v1",
    "model_providers.reflect_go.env_key": "REFLECT_GO_API_KEY",
    "model_providers.reflect_go.wire_api": "responses",
    "model_providers.reflect_go.requires_openai_auth": False,
    "model_providers.reflect_go.supports_websockets": False,
    "model_providers.reflect_go.request_max_retries": 0,
    "model_providers.reflect_go.stream_max_retries": 0,
    "model_providers.reflect_go.stream_idle_timeout_ms": 180000,
    "model_context_window": 16384, "model_auto_compact_token_limit": 14000,
    "model_supports_reasoning_summaries": False, "web_search": "disabled",
    "approval_policy": "never", "features.unified_exec": False,
    "shell_environment_policy.inherit": "core", "allow_login_shell": False,
    "features.plugins": False, "features.apps": False,
}
for key, value in overrides.items():
    cmd += ["-c", key + "=" + json.dumps(value)]
cmd += ["Inspect math.sh and test.sh in this directory. Fix clamp in math.sh so it constrains a number "
        "to the inclusive interval low to high. Use your file/shell tools to make the actual edit, "
        "then run sh test.sh. Do not change test.sh. Report the test result."]
env = dict(os.environ)
for name in ("OPENAI_API_KEY", "ANTHROPIC_API_KEY", "CODEX_API_KEY", "OPENAI_BASE_URL"):
    env.pop(name, None)
env["REFLECT_GO_API_KEY"] = (ROOT / ".build/phase2/provider-key").read_text().strip()
with (ROOT / (".build/phase2/evidence/codex-events-" + label + ".jsonl")).open("w") as out, (ROOT / (".build/phase2/evidence/codex-stderr-" + label + ".txt")).open("w") as err:
    result = subprocess.run(cmd, env=env, stdout=out, stderr=err, timeout=600)
print("Codex exit:", result.returncode)
check = subprocess.run(["/bin/sh", "test.sh"], cwd=work, text=True, capture_output=True)
print("Independent test:", check.returncode, check.stdout)
raise SystemExit(result.returncode or check.returncode)
