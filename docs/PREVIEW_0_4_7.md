# Reflect Go 0.4.7 engineering preview

Open Reflect Go, then open your app. Reflect Go starts its local service and supplies the remembered model selection automatically. Project and model configuration are optional.

## Start

- Mac: open **Reflect Go.app**.
- Windows: open **START Reflect Go.exe**.
- Linux: run `sh "Reflect Go.sh"` from executable storage with a supported runtime and terminal.

If macOS asks for Documents or removable-drive access, allow it once. Phase 4.7 now has a separate macOS app identity from preserved older phases, fixing their competing-permission loop. Rebuilt ad hoc signed previews may still ask again after an update. If an older preview is repeatedly prompting, quit that older copy before reopening Phase 4.7. The native window now waits for a private startup message instead of repeatedly reading protected files.

Keep the phase folder beside the shared **Models** and **Workspace** folders. Earlier phases remain preserved; do not run them on the same provider port simultaneously.

1. If the library is empty, download or import a compatible text GGUF in **Model library**.
2. Click **Open Codex CLI**, **Open Claude CLI**, or **Open OpenCode CLI**. The portable personal package includes these clients. Missing packs produce an explanation before starting the service.
3. Work in the client. Models load when requested; offering several models does not load them all into memory. Client permissions and project/tool workflows remain their own.
4. Finish client work and quit Reflect Go before ejecting. Closing a browser tab does not quit the workstation. Closing the native Mac window does.

A blank project starts in the drive's **Workspace**. To use another folder, paste its path or use **Choose folder…** in the native Mac window. This choice is remembered. A saved path missing on another computer is shown with a warning; clear or replace it before launching. Reflect Go does not install project build tools.

## Models offered to apps

**Models for your apps** lets you offer all suitable models or select a specific set, with an optional starting model. Codex, Claude, and OpenCode CLI can each inherit shared choices or have their own saved selections. Selecting no models explicitly prevents that client from launching. A per-client model selection does not change the shared defaults.

Codex and OpenCode receive model catalogs. Claude Code's launcher prints available model IDs for `/model`; its picker differs from the other clients. Reopen a client after changing its selection. Changing the running service's model set requires stopping it after active work finishes. New model files are discovered on the next service start.

Opening a CLI raises an insufficient stopped-service context to its minimum: 8,192 for Codex/Claude and 16,384 for OpenCode. Other graphical adapters currently require 16,384. The memory screen still applies. Reflect Go refuses to interrupt an active service to change its configuration. **Advanced & performance** holds manual runtime controls and recommendations; **Chat** has its own **Start local chat** action for a smaller setup test.

## More apps

- **DeepSeek Harness:** opens an existing `dsh` installation in its graphical browser interface with drive-local settings and the shared model selection. It is a separate harness, not a model name. Its program/Node runtime are not bundled.
- **OpenCode Web:** opens a graphical interface from the portable OpenCode pack. Its browser session and provider are authenticated locally. It is separate from the native OpenCode Desktop app.
- **Claude Desktop:** experimental macOS adapter for an installed Claude app. Maps up to five shared models to Claude's model slots. It asks before restarting a running Claude session. A connection journal preserves only changed fields and restores them on disconnect or normal Reflect Go shutdown, retaining unrelated settings and reporting conflicting edits. Desktop conversations and tools still require real-app qualification. Choose projects within Claude Desktop. Host desktop history/settings may remain on that computer.
- **Codex in ChatGPT Desktop, OpenCode Desktop, and Antigravity:** visible integration targets, not working Reflect Go launchers yet. Their required mechanisms and limits are recorded in [harness compatibility](HARNESS_COMPATIBILITY.md).

A desktop app's host installation is separate from connecting it to portable inference. Desktop adapters are qualified individually. CLI compatibility does not prove desktop compatibility, and a configurable SDK does not prove its desktop model picker supports local endpoints.

If Reflect Go is interrupted during a Claude connection, reopen the same copy on the same computer and use **Restore previous settings** before reconnecting. Do not delete the restore journal or move the drive to another computer expecting it to restore the original computer's settings. Reflect Go never force-kills Claude or changes its tool approval policy.

## Retained features and packaging

The preview retains resumable verified catalog downloads, GGUF import, memory/context recommendations, filters, streaming basic chat with Stop/New chat, Dark/Light/System, and measured diagnostic timings. One model is resident at a time. Concurrent generation requests receive an explicit busy response; they are not queued or allowed to evict active work. Memory estimates and successful greetings do not establish coding quality.

There are two editions: **Reflect Go** (software and supported portable clients, no weights) and **Reflect Go Starter Pack** (the same plus one small demonstration model). No Plus tier. Public redistribution of the proprietary Claude binary is unresolved, so the shareable packaging script excludes it; the personal assembled copy retains the existing pack.

Build with `scripts/build_preview.py` into **Phase 4.7**. Phase 4.6 and older packages are preserved. Weights remain shared and are not copied into a phase. Source Git includes no weights, client binaries, runtime state, or private logs. Ready-to-download 0.4.7 release assets are not published. See [validation](PREVIEW_0_4_7_VALIDATION.md) for executed checks and remaining release gates.
