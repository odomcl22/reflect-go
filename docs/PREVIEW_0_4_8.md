# Reflect Go 0.4.8 preview

Open Reflect Go, then choose your app. Project folder and offered models are optional choices. Model weights stay on your drive; inference runs using the host computer's memory and processor/GPU.

## Desktop connections

- **Claude Desktop:** maps up to five eligible local models to its model choices. Desktop requests need a large context even for a short greeting. Reflect Go starts at 49,152 tokens, uses a compact Q4 cache for verified compatible model architectures, and limits replies to 4,096 tokens. Unsupported context/model combinations are rejected before connecting.
- **ChatGPT / Codex Desktop:** adds a Reflect Go model catalog and a local Responses router. Existing account credentials are preserved. Local model requests stay local and never fall back to a cloud model. Selecting an existing cloud model still uses its normal provider and account. Start a fresh conversation when switching providers; local remote-compaction is not supported.
- **OpenCode Desktop:** adds a `reflect_go` provider to its JSON/JSONC configuration. Existing providers, comments, permissions and account settings are retained. A custom environment or project configuration can override global defaults.

Native desktop apps keep their own project picker in this preview; the project-folder field applies to CLI/web connectors.

Installed desktop apps are required. These are preview adapters, not bundled desktop applications. macOS app discovery supports Claude, ChatGPT and OpenCode in Applications. Windows discovery supports ordinary per-user/Program Files installations; Microsoft Store installations are not yet qualified. Linux desktop support is limited to an installed `opencode-desktop` executable.

## Disconnect without signing out

Use **Disconnect**, or finish your work and **Quit Reflect Go**. Connected desktop apps close normally, Reflect Go restores the configuration fields it changed, and apps that were running reopen. Existing account credentials are not removed. If a desktop refuses to close or a setting changed externally, Reflect Go reports the problem rather than overwriting it or force-killing the app.

Do not pull out the drive while Reflect Go is running. A force-quit, crash or unplug can prevent restoration; reopen the same package on the same computer and use Disconnect. Portable CLI sessions use isolated configuration; exit them before quitting Reflect Go.

## Downloads

Extract the entire platform ZIP. Keep the launcher next to `bin`, `runtime`, `harnesses`, `config` and `Models`; do not move the app/exe alone.

- Mac: open `Reflect Go.app` (macOS 13.3+, Apple Silicon or Intel).
- Windows: open `START Reflect Go.exe` (64-bit Windows x64 or ARM64).
- Linux: run `sh "Reflect Go.sh"` (x64 or ARM64; compatible libraries and terminal required).

**Reflect Go** includes no model weights. Use Model library to download/import a GGUF. **Starter Pack** adds one small Qwen2.5 1.5B demonstration model for simple chat. That model is not suitable for the desktop adapters' 49K context; use a compatible larger-context model such as Qwen3 4B Instruct 2507 and sufficient memory for desktop work.

Both downloads include portable Codex and OpenCode CLI clients. Claude Code CLI requires separately obtained software; it is not included in these packages. To use Claude Desktop, connect an existing installation on your computer.

Mac public downloads are Developer ID signed and Apple-notarized. Windows preview executables are unsigned and may trigger SmartScreen. Desktop workflow qualification and physical Windows/Linux checks are listed in [validation](PREVIEW_0_4_8_VALIDATION.md).
