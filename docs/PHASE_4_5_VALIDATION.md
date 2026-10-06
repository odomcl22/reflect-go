# Phase 4.5 validation — 2026-10-04

Engineering preview, not Reflect Go 1.0 qualification. Execution host: Apple M1, 16 GiB, macOS 15.7.4. Other operating systems were cross-built and inspected, not physically run.

## Portable clients

Pinned Codex CLI 0.160.0, Claude Code 2.1.289 and OpenCode 1.18.34: native packs for macOS, Windows and Linux, each on x64 and ARM64. Official release/package integrity was checked before extraction. Search and Codex sandbox/app-server helpers are included and hash-checked on connection.

Real local inference used Qwen3-4B-Instruct-2507 Q4_K_M at 16,384 context. All three clients read a fixture marker through their tools. Codex used its normal workspace-write sandbox. Claude and OpenCode ran under an additional acceptance-test macOS sandbox that denied outbound internet and personal-file content outside the disposable fixture and bundle. The launch environment was reduced to OS/terminal essentials and drive-contained profiles; no host account credentials were imported.

A stricter create-file/read-back task has mixed model-quality results:

| Client | Actual tool use and output | Exact-byte acceptance |
| --- | --- | --- |
| Codex | Wrote the marker, ran the shell read-back | Passed; 22.94 s |
| Claude Code | Wrote the marker, ran the shell read-back | Passed; 83.99 s |
| OpenCode | Wrote the marker, ran the shell read-back | Failed: omitted the requested final newline; 38.39 s |

The independent verifier inspected actual file bytes, not the model's claim. The strict workflow suite therefore intentionally remains a recorded failure. See `evidence/phase-4.5/workflows.json` and each client workflow log.

More demanding clamp-function repair attempts were unreliable with this 4B model: Codex described edits without performing them, and OpenCode made incorrect/repeated edits. This release demonstrates protocol and tool integration, not reliable autonomous coding. Users must review changes and test results. An earlier Claude attempt failed because the test proxy dropped its authentication header; the proxy was repaired before the final run. That failed test-proxy run is not evidence of a product authentication failure.

## Automated checks

- Go unit tests with the race detector passed; Mac and Windows-targeted `go vet` passed.
- Tests cover malformed options, paths, profile isolation, pack/helper corruption, authenticated reconnect targets, model screening and lifecycle behavior.
- Six native Go targets compiled; Windows GUI and console variants compiled. The root Windows selector is a small PE32 GUI executable that selects the native x64 or ARM64 executable using the OS architecture.
- Duplicate browser launch reconnected to the same authenticated workstation. Restart replaced a stale session after force termination. Quit removed reconnect credentials and exited. See `evidence/phase-4.5/reopen.json`.
- The actual Swift controller passed shared-model discovery, corrupt-settings recovery, model eligibility/selection, occupied-port failure without stopping the foreign listener, real 1.5B generation, measured speed/memory, duplicate-app locking, stop/restart, 4B model switching and engine cleanup after a simulated app crash. The 1.5B sample measured 48.01 tokens/s; this is one measurement, not a performance guarantee. See `evidence/phase-4.5/native-controller.txt`.
- Final local qualification passed software/model integrity, real Metal inference, rejected unauthenticated requests, authenticated API/metrics, streaming, generation and shutdown. See `evidence/phase-4.5/qualification-local.json`.
- Mac app and its engine helper have ad-hoc signatures. This is not Developer ID notarization or Windows publisher signing.

## Platform limits

The user's Windows x64 test on a 31.9 GiB PC with an RTX 4070 loaded Qwen3 4B through Vulkan in about six seconds and generated 23 tokens at about 33 tokens/s. All three CLI terminal windows reportedly opened and immediately closed. The engine log also contains unidentified unauthorized requests; it does not identify which client or request caused them. This is evidence of working inference, not successful CLI qualification.

The October 4 maintenance fix restores console standard handles in the Windows connector before launching the CLI. The GUI launcher previously supplied NUL streams through Go's default subprocess behavior, which also made its exit pause read EOF immediately. Client launches now save a stage/error/exit-status report and identify HTTP 401/403 failures separately. Mac Go tests pass, including launch-error persistence, key redaction and authentication preflight tests. The Windows console regression test reproduces the GUI launch and checks inherited console handles; it is cross-compiled here, not executed on Windows. The updated package needs a physical Windows CLI retest. This working build is newer than the original archive.

Linux, Intel Mac and ChromeOS still need physical tests. A successful build and dependency audit do not certify launch, terminal handling, GPU drivers or sandbox setup on those hosts.

Windows x64 client/helper imports use OS DLLs. ARM64 ripgrep also imports `VCRUNTIME140.dll`, which is not bundled; that platform needs an existing compatible Microsoft runtime. Linux inference retains Phase 4's glibc 2.38+ floor; ARM64 ripgrep needs the usual `libgcc_s.so.1`. See `evidence/phase-4.5/dependencies.json` for the binary audit. No inference stack, Node, Python or Go installation is required to launch the packaged software; project build tools remain host dependencies.

During local Mac GUI testing, macOS privacy checks stalled an external helper/script under Documents. The app now embeds and signs its ownership helper and starts the native engine without a shell wrapper. A diagnostics file read also exposed a UI freeze under the same OS access delay; log reads now run in the background, and navigation and Quit were verified while the check was pending. Utility timeouts include a final stop for a helper stalled before its own cleanup code starts. A fresh-account macOS permission flow still needs qualification. No privacy or security policy was changed.

## Release layout

The personal dist and USB builds contain all three requested clients. Models remain only in the shared sibling Models folder. Prior phases are preserved. The only backup location is the source project's `releases/` folder.

The backup archive excludes weights, runtime/client state and proprietary Claude binaries. Its client index and integrity manifest are adjusted to match that archive. Pinned official Claude download metadata and preparation scripts remain available for restoring a personal copy. No public redistribution right is asserted.
