# Phase 3 — portable AI workstation

Build a native macOS application around the proven portable provider. Keep Phase 1 and Phase 2 usable; keep models in one shared library. Archive source and phase software only in the source project's `releases/`, without model weights or state. The distribution and USB contain working phases, not backup archives.

The main window must show detected CPU/Metal/RAM, available models with context-sensitive memory estimates, engine load/unload controls, measured performance and resident memory, client launch controls, context/compute settings, and useful diagnostics. Include the startup sequence and Claude's supplied-key confirmation in the app itself. The model remains local; clients retain their tools and interface.

Use the existing native launcher and pinned llama.cpp runtime. Add a structured inventory command rather than duplicate model validation in the UI. AppKit hosts SwiftUI, with no web server for the UI, browser dependency, Electron package, Python, or installation on the destination Mac. The app owns only its own engine process. Default Phase 3 port 8766 leaves Phase 2's 8765 available.

Acceptance:

- Open the actual app from the assembled phase and from the USB; visually inspect the real UI.
- List actual shared models and hardware; update model estimates as context changes.
- Start a model, wait for authenticated readiness, generate a real response, and display measured speed and memory.
- Stop/unload, restart, and quit through the UI; listener and child process must exit.
- A busy port, invalid configuration, missing model/runtime, or integrity failure must produce a useful error without affecting other processes.
- Launch the existing Codex and Claude connectors from a selected project without modifying host account configuration or putting keys into generated scripts.
- Preserve the user's active Phase 2 engine throughout testing.
- Check native dependencies, bundle hashes, archive contents, and absence of duplicated archive directories/models across phases.

Limits: Apple Silicon macOS 13.3+ only. Existing client installations are required. Small-model coding reliability and second-Mac testing remain open. Process resident memory is not total GPU allocation. No cross-platform, LAN, or distributed work in this phase.
