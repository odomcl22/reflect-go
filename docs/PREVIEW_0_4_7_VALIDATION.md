# Reflect Go 0.4.7 validation

Date: 2026-10-10. Engineering preview; physical cross-platform qualification remains incomplete.

## Implemented

- App-first home screen; CLI click starts and waits for the local service automatically.
- Optional remembered project, native Mac folder picker, shared and per-CLI offered models/defaults.
- Separate saved shared settings and effective gateway model union; frozen per-launch client catalogs.
- Integration inventory with explicit availability; installed DeepSeek Harness and portable OpenCode Web adapters.
- Experimental Claude Desktop profile journal and model-slot gateway with restoration/conflict protection.

## Executed checks

- Full Go regression suite with race detector, `go vet`, and six platform test-binary compilations passed (Mac/Windows/Linux, Intel/ARM). Windows targets were recompiled after process-cleanup changes. The six packaged binaries and universal native Mac window compiled.
- One-click tests cover prerequisite failures, duplicate/concurrent clicks, cancellation, preserving active sessions, independent shared/per-client choices, explicit empty selections, frozen catalogs and defaults, missing remembered folders, and context adaptation.
- Temporary-profile tests cover preserving unrelated Claude settings, foreign host/home rejection, conflicting edits, rollback and retry. Loopback proxy tests cover authentication, origins, slot catalogs, model routing, stripped headers, unsupported tools and malformed requests. Integration-sequencing tests verify validation before app restart, cancellation rollback, and restore ownership checks. No actual user desktop profile was edited.
- Installed DeepSeek Harness 0.1.1-rc.2 served its real browser interface and selected-model API using an isolated drive-local profile. Portable OpenCode 1.18.34 served its real graphical interface and selected local provider catalog. Browser-bridge authentication and cross-origin rejection tests passed. These startup/catalog checks do not establish inference/tool workflow quality.
- Native UI test on the Apple M1 Mac: Apps layout rendered, project folder picker opened and returned the selected path, and clearing the project used the portable Workspace. Clicking Codex from a stopped service started the gateway, loaded Qwen3 4B and completed the no-tools greeting request with a clean client exit report. The runtime processed 3,004 prompt tokens and generated 3 output tokens. Terminal content itself was not inspected. OpenCode Web launch also reported success. Normal quit stopped the owned gateway, model runtime and web server.
- Final UI fixes passed JavaScript syntax and mocked interaction regressions for failed initial inventory/retry and automatic context synchronization (8K form to 16K backend). Native startup guidance and the populated Apps screen rendered in the final build. The [saved screenshot](media/reflect-go-apps-047.png) is from that running native app. Packaging exclusion tests and whitespace checks passed.

## Qualification boundaries

No real Claude Desktop user profile was changed to validate this adapter. Profile and protocol tests are not desktop workflow qualification. Codex/ChatGPT Desktop, native OpenCode Desktop and Antigravity adapters remain unavailable.

The existing 0.4.6 real local inference and three-CLI greeting checks are recorded in [the preserved validation report](PREVIEW_0_4_6_VALIDATION.md). Physical Windows, Intel Mac, Linux and conditional Chromebook qualification, representative coding tasks, signing/notarization, public client distribution and published release assets remain outstanding.

## macOS preview identity

Launching the rebuilt app from Documents waited for macOS Files and Folders approval. The TCC log showed that the new ad hoc code hash did not match the previous permission record, then requested Documents access. The backend waited before Go started. The temporary test copy opened successfully. The native window now explains the permission request while starting, allows up to two minutes, and gives recovery guidance on timeout. No privacy settings were reset. Stable release signing and real removable-drive/Finder launch qualification remain required; a successful temporary-folder test does not close that gate.

Abrupt process death and forced drive removal need further lifecycle testing for the additional graphical harnesses. Normal owned-process shutdown was tested. Public assets remain unpublished.
