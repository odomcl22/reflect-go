# Reflect Go 0.4.6 validation

Date: 2026-10-10. Engineering preview; physical cross-platform qualification remains incomplete.

## Implemented

- Optional-project client launch with a separate default portable Workspace.
- Authenticated multi-model gateway with stable IDs, on-demand loading, one resident model, active-request exclusion, cancellation, and explicit unknown-model errors.
- Codex model catalog, OpenCode provider catalog, and Claude custom model IDs without unsupported reasoning-level claims.
- Curated resumable model downloads with integrity verification, disk-space checks, cancellation, shared-library writer locking, and GGUF import.
- Model filters and explicit hardware/context recommendations.
- Shared Mac/Windows/Linux workstation, native Mac window, streaming basic chat, stop/new conversation, and portable Dark/Light/System preference.
- Gateway/launch status, load and first-response timings, and reviewed diagnostic copy.

## Executed checks

- Go regression suite with race detector: passed, including authenticated two-model routing, no load on catalog reads, busy exclusion, cancelled-load cleanup, resumed downloads, checksum rejection, import boundaries, default workspace, and Codex task argument placement.
- Six Go OS/architecture targets compiled: macOS ARM64/x64, Windows ARM64/x64, Linux ARM64/x64. Native universal Mac window compiled and ad-hoc signed.
- Real inference on the available Apple M1 Mac: service available before a model is loaded; streamed one response each from the existing Qwen2.5 1.5B and Qwen3 4B models; confirmed the requested model was active after each request; rejected unknown model IDs.
- Chat and coding recommendations returned usable starting selections on this host. Dark/Light/System API round trips passed.

- Codex, Claude Code, and OpenCode returned real local greeting responses using portable profiles without a selected project. OpenCode completed at 16,384 context with Qwen3 4B; an 8,192-context test looped in compaction, so this preview explicitly rejects OpenCode launches below 16,384. Greetings establish connectivity, not coding quality.
- The native Mac window rendered the shared UI, restored Dark appearance, streamed a reply, opened the native import picker, and displayed the Stop confirmation. Testing caught WebKit stream-cancellation and deferred-window-termination problems, with explicit cancellation and exit-notification fixes added. Retests passed: Stop response showed its stopped state, a new chat succeeded immediately afterward, and closing the window terminated the app, UI backend, gateway and runtime with the provider port closed.
- Software packaging exclusion tests passed: no model weights in model-free archives, no runtime/client state, and no proprietary Claude pack or stale installed-pack entry.

Compilation is not physical qualification. Synthetic download tests exercise the transfer and integrity protocol without downloading duplicate gigabytes of weights.

## Remaining qualification and product work

- Native launch from the protected Documents folder stalled in file access on this host; the temporary-folder copy worked. Retest normal drive/Finder launch and macOS permission handling before claiming distribution readiness.
- Physical Windows CLI console regression retest, Intel Mac, Linux hardware/runtime and permissions tests, and conditional Chromebook tests.
- Representative coding tasks across client/model/context combinations; a successful greeting is only a connection check.
- Public client acquisition/distribution decision, signed/notarized release packaging, and published software/starter assets.
- User studies, improved macOS available-memory measurements, deeper model-capability validation, and optional alternative engine benchmarks.

The implementation does not establish that all roadmap acceptance gates are complete. Existing roadmap performance research and deferred desktop/network extensions remain deferred.
