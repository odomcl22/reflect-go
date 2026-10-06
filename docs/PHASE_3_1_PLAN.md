# Phase 3.1 — workstation hardening

Scope: harden the current Apple Silicon Mac workstation. Preserve Phase 1, 2, and 3 working folders, share their existing Models library, and archive only source/software in the project's root releases directory. Windows, Mac, Linux and conditional ChromeOS support are explicit Phase 4 targets, not claims for this release. Codex, OpenCode and Claude portable packs remain Phase 4.5.

Implementation:

- A small native supervisor owns one direct engine child. The app holds a pipe open; closing it or losing the app stops that child, escalating after eight seconds. No PID from disk or process-wide kill is used.
- Keep engine memory measurements attached to the inference child, not the supervisor.
- Bound inventory/integrity checks and startup; explain failed health checks and leave a stop control available.
- Preserve the selected model when context makes it ineligible. Explain that condition before starting; notice missing models and invalid saved settings.
- Hold a session lock before rotating logs. Retain one previous log, guard file integrity checks against concurrent inference, and document the lifecycle in Quick start.

Acceptance: real supervisor process tests for normal stop, app death, cancellation during launch, stubborn children, timeouts, direct-child exit, and unrelated-process survival; exercise Workstation state against real inventory and inference; inspect the actual UI; verify package signatures/hashes and model-free archives. Deploy as a new Phase 3.1 USB folder without copying model weights. Record untested physical-machine/OS cases explicitly.
