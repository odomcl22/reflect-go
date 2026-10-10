# Reflect Go 0.4.6 engineering preview

This preview adds the client-first workflow from the [roadmap](ROADMAP.md). It is not a 1.0 release or a claim of qualification on every packaged platform.

## Start

- macOS: open **Reflect Go.app**. The native window now hosts the shared workstation interface.
- Windows: open **START Reflect Go.exe**.
- Linux: run `sh "Reflect Go.sh"` from executable storage with a supported runtime and terminal.

Keep the phase folder beside the shared **Models** folder. No model weights are copied into a phase. Do not run old and new phases on the same provider port at the same time.

1. In **Model library**, download a catalog model or import an existing single-file text GGUF. The starter model is a chat/setup demonstration, not a coding-quality promise.
2. In **Workstation**, choose Chat or Coding and click **Recommend settings**. Review the explanation, enable the models to expose, and choose a default.
3. Click **Start engine**. This starts the local model service without preloading every model. The selected default loads only when requested.
4. Click **Open Codex CLI**, **Open Claude CLI**, or **Open OpenCode CLI**. A project is optional; blank uses **Workspace**, beside Models and the phase folders. Use **Open with a project or task…** for a specific folder. The host still supplies your project's build tools.
5. Choose a model inside Codex or OpenCode. Claude's launcher prints the available IDs for its `/model` command; it does not have an identical arbitrary-model picker. Reopen a client after changing the engine's enabled models.
6. Finish client tasks, stop the engine, and quit before ejecting. Closing a browser tab alone does not quit its launcher. Closing the native Mac window requests shutdown.

OpenCode requires at least 16,384 context tokens in this preview: testing at 8,192 caused a repeated compaction loop. Codex and Claude require at least 8,192. If memory cannot support that, use the built-in Chat with a smaller context.

## Models and memory

The engine exposes stable model IDs. It loads one model at a time and unloads it when a request selects another. While a response is active, concurrent generation/model-switch requests receive HTTP 409 with a retry hint; they do not evict the active model. The next request can retry after the response finishes or is cancelled.

The model catalog is a snapshot at engine startup. After a download or import, restart the engine and reopen clients. Requests for an unknown or disabled model fail explicitly. The legacy `reflect-go-local` alias resolves to the default for older integrations.

Recommendations use the existing conservative half-RAM screen, GGUF memory estimates where available, and automatic backend probing. Windows and Linux recommendations additionally use reported available memory when present; macOS retains the conservative estimate. These are starting settings, not fit guarantees. They do not prove model architecture compatibility, or qualify coding reliability. Imported models are unverified until successfully loaded and tested. No unsupported reasoning-effort choices are advertised; this preview uses the runtime's existing non-thinking configuration.

## Download and import

The small catalog pins source revisions, filenames, sizes, licenses, and SHA-256 values. Downloads go directly to Models with a `.partial` suffix. Download/resume and Pause are explicit actions. The final filename appears only after size, checksum, and header checks. A failed checksum removes the damaged partial so the next attempt starts cleanly. Existing models are never overwritten.

Filters cover purpose, installed status, download size, and conservative fit at 2,048 context. The catalog's fit filter uses a conservative fallback estimate; the installed-model screen can use actual GGUF metadata. Neither is a live free-memory measurement. Import copies a chosen GGUF and checks its header, not its publisher authenticity. Internet is needed for downloads; prepared local inference works offline.

## Chat, appearance, and diagnostics

**Chat** streams real replies through the same provider used by clients. It supports model selection, Stop response, New chat, and measured time to first response. It has no file, terminal, or web tools. Conversation text stays in this window's memory; reload/New chat clears it. Engine diagnostics may still contain prompts. CLI clients retain their own portable session storage.

Choose **Dark**, **Light**, or **System** in the sidebar. The choice is stored on the drive. Diagnostics includes client launch reports, gateway loading state, model load time, measured generation speed, and available runtime resident-memory readings. Review reports before sharing. Raw logs are separate from the copied report.

## Packaging and status

The intended downloads remain **Reflect Go** (software and supported clients, no weights) and **Reflect Go Starter Pack** (the same functionality plus one small test model). The personal assembled preview includes the existing three CLI packs. Public redistribution of Claude's proprietary binary remains unresolved; shareable archives must exclude it unless an appropriate distribution method is established.

Earlier phases are preserved. New software is assembled into Phase 4.6 with `scripts/build_preview.py`; source Git contains no weights, client binaries, runtime state, or private logs. See [validation](PREVIEW_0_4_6_VALIDATION.md) for actual checks and remaining gates.
