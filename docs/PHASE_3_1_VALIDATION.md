# Phase 3.1 validation — 2026-10-04

Test machine: Apple M1, 16 GiB unified memory, macOS 15.7.4. Engineering target remains Apple Silicon/macOS 13.3+. No new inference runtime or model download; the pinned runtime and shared 1.5B/4B weights are unchanged.

## Passed locally

- 21 native launcher operational/failure tests: model/config validation, shared paths, manifest confinement, malformed downloads, relocation, invalid ports and runtime arguments.
- Seven real supervisor tests: child exit/output/status, pipe closure, SIGTERM, abrupt owner SIGKILL, immediate cancellation (12 launches), timeout and a child ignoring SIGTERM. Stubborn-child shutdown escalated after eight seconds. An unrelated process survived.
- Native Workstation controller against real models: corrupt-preference notice; shared model inventory; 4B selection retained and Start disabled at 32K context; busy-port error with foreign listener intact; 1.5B generation; measured child memory and speed; duplicate-app session lock; immediate stop/restart; switch to 4B at 16K; previous-log preservation; force-killed controller released the real 4B listener.
- The restart test exposed TIME_WAIT sockets being mistaken for an occupied listener. The preflight now uses SO_REUSEADDR, matching the server. Immediate restart and actual occupied-port rejection both passed afterward.
- Actual native GUI inspected: real hardware/models, 4B load, authenticated readiness, response, Stop & unload confirmation, stopped state, context change with selected ineligible model visibly retained, default context restored, Quick start and normal menu quit. The GUI displayed 22 generated tokens, 21.2 tokens/second and 4.6 GiB engine resident memory on one 4B response. These are measurements from this session, not performance guarantees.
- Local provider checks: unauthenticated model request 401; authenticated models and metrics 200; unrelated web Origin not allowed. Host remains loopback-only.
- The rebuilt GUI initially encountered a macOS loader/file-access delay. Its independent 30-second inventory deadline produced an actionable error; Refresh then discovered both models. No privacy/security setting was changed. Allow normal macOS file-access prompts when shown. First-open behavior on a fresh account remains unqualified.

## Packaging and USB

- Deployed a new `Phase 3.1` folder to the existing exFAT Reflect Go drive. All earlier phase folders remain present. No models or state were copied by deployment; both models remain in the shared Models directory.
- Local native integrity check passed all 59 packaged files, including the pinned 4B model. USB deployment checked shared model hash and byte-compared packaged software. Ad-hoc app signature verification passed on the local and USB copies.
- USB `VerifyPortability.command` passed integrity/hardware checks and generated the correct library-hours summary with networking denied. The runtime offloaded 29/29 layers to Metal; this 1.5B run measured 48.85 tokens/second, 64.58 seconds total including loading. Evidence remains in `Phase 3.1/state/portability-check.O1ZqAQ/` on the USB and is excluded from the archive.
- Actual USB app opened and discovered the shared models. It loaded the 1.5B model at 16K context, generated a response (46.3 tokens/second and 1.6 GiB resident in that session), and quit through **Stop and quit** while the engine was active. App, supervisor, engine and listener exited. The normal 4B/16K selection was restored afterward.
- Backup destination is only the source project's `releases/`. Archive source/software exclude model weights, state and credentials. Dist and USB hold runnable phase folders rather than nested backup directories.

## Limits

Windows, Linux, Intel Macs and ChromeOS are not implemented or qualified by this release; they belong to Phase 4's explicit hardware/OS matrix. ChromeOS depends on an enabled, permitted Linux environment and needs physical tests for drive execution and acceleration. No second physical Mac or clean user account was available.

The five-minute startup deadline and repeated-health-failure message are implemented; the full five-minute stalled-model scenario was not separately exercised. Utility timeout and recovery were exercised. OS scheduling, suspended processes, inaccessible drives and OS permission dialogs can delay cleanup. Force-killing the supervisor itself or the whole process tree is outside the app-crash protection. Separately started provider terminals intentionally keep running after the GUI closes.

Client connectors and model weights are unchanged; no new claim of coding reliability is made beyond Phase 2/3 evidence. Codex CLI, OpenCode CLI and Claude Code CLI portable packs remain Phase 4.5. No desktop client, network orchestration, or distributed inference was added. The app remains ad-hoc signed, not Developer ID signed or notarized.

## Reproduce

- `python3 tests/test_bundle.py 'dist/Reflect Go/Phase 3.1'`
- `python3 tests/test_engine_host.py 'dist/Reflect Go/Phase 3.1/bin/engine-host'`
- Compile `Sources/Workstation.swift` and `tests/WorkstationAcceptance.swift` with Swift's `-D WORKSTATION_TEST -parse-as-library`, then run `python3 tests/workstation_acceptance.py` on this 16 GiB test profile. It creates and removes an isolated software-only phase next to shared Models and uses test port 18876.
- Run `VerifyPortability.command` from a destination phase for integrity, hardware and network-denied inference evidence.
