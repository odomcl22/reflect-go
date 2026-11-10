# Portable coding clients

Reflect Go's personal Phase 4.8 collection contains native Codex, Claude Code,
and OpenCode CLI packs for macOS, Windows, and Linux on x64 and ARM64.
Clients and their Reflect Go profiles stay in the portable collection. A terminal,
host permissions, and any development tools required by your project are still
needed. An included coding client does not make every local model capable of
reliable tool use.

## Add Claude Code

Reflect Go 0.4.9 public packages include Codex and OpenCode. To add Claude Code:

1. Open Reflect Go and click **Get Claude Code**.
2. Confirm the one-time download from Anthropic.
3. Wait for verification, then click **Open**.

The native downloader keeps Claude Code in **Reflect Go Files/harnesses** and
verifies the pinned publisher archive digest before installation. It requires
internet for setup, but no Python, Node, shell-profile changes, or installation
into your home directory. Anthropic's license and terms continue to apply.
The public package provides the connection and downloader rather than
redistribute Claude's proprietary executable. The pinned CLI is 2.1.289, matching
this preview's connectors; this does not claim it is the newest release.

An optional terminal tool remains available in **Reflect Go Files/tools**. Quit
Reflect Go, open a terminal in **Reflect Go Files**, and run
`python3 tools/add_claude_cli.py` (Windows: `py -3 tools/add_claude_cli.py`).
This fallback requires Python 3. The in-app downloader does not.
Previously published 0.4.8 ZIPs are preserved and do not acquire the new downloader
automatically.

For another supported computer, add `--platform windows-x64`,
`--platform windows-arm64`, `--platform macos-arm64`, `--platform macos-x64`,
`--platform linux-x64`, or `--platform linux-arm64` before moving the drive.
Every package's Claude download list includes all six targets. This stages the
CLI only; the destination computer also needs its matching Reflect Go launcher
and inference runtime.
Windows/Linux execution still needs physical-device testing; a verified download
is not proof of a working coding workflow on those machines.

Developers preparing the personal collection from source can run:

```sh
python3 scripts/prepare_harnesses.py --bundle "dist/Reflect Go/Phase 4.8" --clients claude
```

This reuses verified installed packs and adds any missing pinned platforms.
Public packaging continues to exclude the personal Claude executables.

Sources: [Claude Code setup](https://code.claude.com/docs/en/setup),
[Claude Code license](https://github.com/anthropics/claude-code/blob/main/LICENSE.md).
