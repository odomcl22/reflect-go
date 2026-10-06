#!/usr/bin/env python3
"""Exercise the installed Claude Code agent against the local provider."""
import json
import os
from pathlib import Path
import shutil
import subprocess
import sys

ROOT = Path(__file__).resolve().parents[1]
label = sys.argv[1] if len(sys.argv) > 1 else "4b"
if not label.replace("-", "").isalnum():
    raise SystemExit("Use an alphanumeric label")
work = ROOT / (".build/phase2/claude-project-" + label)
if work.exists():
    raise SystemExit("Choose a fresh test label; existing evidence is preserved")
shutil.copytree(ROOT / "tests/fixtures/agent-project", work)
env = {k: v for k, v in os.environ.items() if not k.startswith(("ANTHROPIC_", "CLAUDE_", "OPENAI_"))}
env.update({
    "ANTHROPIC_API_KEY": (ROOT / ".build/phase2/provider-key").read_text().strip(),
    "ANTHROPIC_BASE_URL": "http://127.0.0.1:8765",
    "ANTHROPIC_MODEL": "reflect-go-local",
    "ANTHROPIC_DEFAULT_HAIKU_MODEL": "reflect-go-local",
    "ANTHROPIC_SMALL_FAST_MODEL": "reflect-go-local",
    "CLAUDE_CODE_DISABLE_NONESSENTIAL_TRAFFIC": "1",
    "DISABLE_PROMPT_CACHING": "1",
    "CLAUDE_CODE_MAX_OUTPUT_TOKENS": "2048",
    "MAX_THINKING_TOKENS": "0",
    "NO_PROXY": "127.0.0.1,localhost",
})
cmd = [shutil.which("claude"), "--bare", "--setting-sources", "", "--strict-mcp-config",
       "--mcp-config", '{"mcpServers":{}}', "--disable-slash-commands", "--no-chrome",
       "--tools", "Read,Edit,Bash,Glob,Grep", "--allowedTools", "Read", "Edit", "Bash(sh test.sh)",
       "--permission-mode", "dontAsk", "--model", "reflect-go-local",
       "--print", "--no-session-persistence", "--output-format", "stream-json", "--verbose",
       "Inspect math.sh and test.sh in this directory. Fix clamp in math.sh so it constrains a number "
       "to the inclusive interval low to high. Use your file tools to make the actual edit, "
       "then run sh test.sh. Do not change test.sh. Report the test result."]
evidence = ROOT / ".build/phase2/evidence"
with (evidence / ("claude-events-" + label + ".jsonl")).open("w") as out, (evidence / ("claude-stderr-" + label + ".txt")).open("w") as err:
    result = subprocess.run(cmd, env=env, cwd=work, stdout=out, stderr=err, timeout=600)
check = subprocess.run(["/bin/sh", "test.sh"], cwd=work, text=True, capture_output=True)
assert (work / "test.sh").read_bytes() == (ROOT / "tests/fixtures/agent-project/test.sh").read_bytes()
print("Claude Code exit:", result.returncode)
print("Independent test:", check.returncode, check.stdout)
raise SystemExit(result.returncode or check.returncode)
