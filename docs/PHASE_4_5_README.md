# Reflect Go — Phase 4.5

This personal engineering preview carries the inference engine and native Codex CLI 0.160.0, Claude Code CLI 2.1.289 and OpenCode CLI 1.18.34 packs. No Node, Python, Go or separate inference installation is needed to start them. Keep this entire folder beside the shared Models folder.

## Start here

**Windows:** double-click **START Reflect Go.exe**. Your default browser opens the workstation. Double-clicking it again reopens the running workstation; it does not start a second model. There is no installer and no command window to keep open. Finish with **Quit workstation** in the page. Closing only the browser tab leaves the workstation running so it can reconnect. The old command launchers are in **Advanced** for troubleshooting.

**Mac:** open **Reflect Go.app**. Finish by stopping the engine and quitting the app. A native-app drive-check timeout remains in the automated Mac test: if normal macOS access approval and Refresh do not resolve it, quit the app and open **Browser Workstation.command**. Keep that terminal open and finish with **Quit workstation** in the page. See DEVICE_TEST.md for the final USB test results.

**Linux:** run `sh "Reflect Go.sh"` from this folder. Keep that terminal open while using the browser workstation and finish with **Quit workstation**. ChromeOS remains conditional on its Linux environment, executable storage, compatible architecture and device policy.

Choose the Qwen3 4B model, **16,384 tokens**, and **Automatic** compute. Start the engine and wait for readiness. Choose a project outside the Reflect Go collection, then click **Open Codex**, **Open Claude**, or **Open OpenCode**. Each opens its CLI in a terminal; use one client at a time. In the browser interface, paste the full project path into its project box (on Windows, copy it from File Explorer's address bar). Leave the browser workstation's Task field empty for interactive chat. Its optional task field submits a single noninteractive request; actions needing a prompt may be denied in that mode.

Claude may ask whether to use the supplied API key and recommend No. Choose Yes for this connection to Reflect Go's local provider. Normal project and tool permission prompts remain in place. No cloud model fallback is configured. Model capability still matters: a small local model can misunderstand tools or describe changes it did not make; inspect actual changes and test results.

For a lighter inference-only test, use the 1.5B model with 2,048 context and **Try a quick response**. Coding connectors require at least 8,192 context; 16,384 is the tested setting. A 2,048-token chat test is not a coding-harness test.

## Compact model library update — October 4

The personal USB now has Bonsai 8B Q1_0, Qwen3.5 4B IQ4_XS and Qwen3.5 9B IQ4_XS in its shared Models folder. See the collection README for 8/12/16/24 GB starting settings. The working Phase 4.5 Mac and browser interfaces now allow ten minutes for a slow model load; the 9B model took just over five minutes on this stick. This maintenance change is newer than the original Phase 4.5 archive.

Use the workstation/API for Qwen3.5. Its newer template is not compatible with the older standalone terminal chat startup. Simple API generation passed on the 16 GB M1; tests on other machines and coding tool use remain separate.

## What travels

- `harnesses/`: native clients for Mac, Windows and Linux on x64 and ARM64, plus search and Codex helper programs. The default connector checks the selected client/helper hashes and uses the drive copy. It never silently substitutes a host installation.
- `state/harnesses/`: separate client/OS profiles, settings, caches and sessions. Host API keys and user configuration are not imported. Do not share this folder as a release.
- `../Models/`: the existing shared weights. This release adds no duplicate model files.
- `runtime/`: the Phase 4 inference packs. Your computer provides CPU/GPU, RAM, OS libraries, browser/terminal and project build tools.

Mac requires macOS 13.3+. Windows requires 64-bit Windows 10/11; native ARM64 search currently requires the existing Microsoft Visual C++ runtime (`VCRUNTIME140.dll`). Intel/AMD Windows CLI binaries and search use OS DLLs. Windows GPU acceleration retains Phase 4's driver/runtime prerequisites; Automatic can use the self-contained CPU engine. Codex's Windows sandbox may require its own initial OS permission/setup flow; Reflect Go does not disable it. Windows and Linux execution remain device-test requirements, not conclusions from cross-compilation.

Linux inference retains the glibc 2.38+ baseline from Phase 4. ARM64 ripgrep also needs `libgcc_s.so.1`; ordinary supported distributions supply it. Linux sandbox tools require the OS to permit their facilities. Project compilers, Git workflows and other development tools are not all bundled.

## Recover a window

The October 4 Windows CLI maintenance fix reconnects a newly opened terminal to its keyboard and screen. Previously, the workstation could launch clients with disconnected input/output, making their windows close immediately. Retry from **START Reflect Go.exe**, wait for the engine to be ready, select your project folder, and open one CLI at a time. Leave Task empty for an interactive session.

When a CLI ends, its terminal now waits for Return so you can read its message. Its latest launch stage, startup error or exit status is saved in `state/client-launch-codex.json`, `state/client-launch-claude.json`, or `state/client-launch-opencode.json`. These reports do not capture the conversation or the CLI's screen output. If a window still closes immediately, send its launch report; if it stays open with an error, also save that message. A connection-key rejection is now identified separately from an engine that is not ready. Stop and restart the engine in this phase if that rejection appears.

On Windows, open **START Reflect Go.exe** again. If the default browser cannot open, the launcher shows a message with the reconnect file: `state/OPEN-WORKSTATION.html`. If Windows itself blocks execution, save its exact message; do not change device security settings as a troubleshooting guess.

For errors, use Diagnostics in the workstation. If necessary, open `Advanced/Reflect Go.cmd` on Windows to keep a diagnostic console. A busy API port means another phase/provider is using it; finish that session or select a different port under Advanced run settings. Existing Phase 4 sessions are separate from Phase 4.5.

## Verification

After quitting the workstation, run **Verify Portability.command** on Mac, **Advanced/Verify Portability.cmd** from a Windows Command Prompt, or **Verify Portability.sh** on Linux. Optional arguments `--model qwen2.5-1.5b-instruct-q4_k_m.gguf --context 2048` select the lighter test. `--backend cpu` tests CPU explicitly. This checks packaged files and real inference, authentication, streaming and shutdown; it does not run coding clients or certify another device.

The report is `state/qualification.json`. Copy/rename it after each host/backend test so the next run does not overwrite it. Review logs before sharing; send the qualification report rather than the whole state directory.

## Packaging and upstream references

Codex and OpenCode retain their upstream Apache-2.0/MIT licenses. Claude Code is proprietary and subject to [Anthropic's terms](https://github.com/anthropics/claude-code/blob/main/LICENSE.md). This personal drive copy was prepared from Anthropic's published native packages. The release archive excludes Claude binaries and all client state; public redistribution permission is not asserted. Restore a personal Claude pack from the pinned official source using the source archive’s preparation script before expecting its button to be available. For example, from its `source` folder, run `python3 scripts/prepare_harnesses.py --bundle "/path/to/Reflect Go/Phase 4.5" --platforms macos-arm64` on an online development Mac. Choose `windows-x64`, `windows-arm64`, `macos-x64`, `linux-x64`, or `linux-arm64` as appropriate; the script updates the integrity index and does not require recompilation.

Configuration references: [official OpenAI documentation](https://developers.openai.com/codex/config-advanced), [Claude gateways](https://code.claude.com/docs/en/llm-gateway), and [OpenCode providers](https://opencode.ai/docs/providers/). Versioned download URLs and integrity hashes are in the included lock files. See VALIDATION.md for tested behavior and remaining qualification work.
