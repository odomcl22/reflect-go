# Reflect Go 0.4.8 validation

Checked October 10, 2026 on an Apple M1 Mac with 16 GiB RAM. Availability, a compiled artifact, a protocol test, and a qualified desktop workflow are different levels of evidence.

## Completed checks

- Complete Go test suite passed. New tests cover local/native routing, gzip/zstd requests, account-header isolation, context limits, bounded Claude output, JSON/TOML profile restoration, JSONC comments, temporary credential cleanup and concurrent-setting protection. Existing restart consent, cancellation and rollback tests also pass.
- Installed ChatGPT desktop app-server accepted the generated model catalog and completed a real local turn through the private Responses router (38 seconds including 2,671 prompt tokens). A temporary `CODEX_HOME` kept the live desktop app and account unchanged.
- Claude desktop model alias and effort normalization returned a real local `OK` reply through the Anthropic endpoint. This protocol test did not restart the user’s open Claude window.
- OpenCode 1.18.34 loaded the global `reflect_go` provider, local URL and default model from a temporary home. The native installed GUI is a separate qualification step.
- Qwen3 4B Instruct 2507 Q4_K_M loaded on Metal with a 49,152-token context and Q4_0 key/value cache. The small Qwen2.5 1.5B model is correctly excluded at that context because its declared maximum is 32,768.
- Preview build/packaging tests: 8 passed. Downloader tests: 8 passed. Native engine-owner tests: 7 passed. Historical launcher tests: 21 passed against their matching Phase 3.1 binary. The historical scripts require explicit executable arguments; generic unittest discovery is not their supported invocation.
- Compiled Go engines for macOS, Windows and Linux on x64 and ARM64. Compiled a universal native Mac window and Windows graphical launcher. Mac Info.plist references the ICNS; Windows executable extraction confirms an embedded icon group.
- Public packaging filters private state, credentials, other-platform binaries, personal models and proprietary Claude CLI binaries. It checks source hashes and generates platform manifests. The launcher is at the top of the extracted package.

- Apple accepted notarization for the public Mac package. The app ticket was stapled and validated; Gatekeeper reports `accepted` / `Notarized Developer ID`. Vendor Codex/OpenCode signatures are retained.

- Signed backend startup handshake and authenticated Quit passed. A 33,846-token artificial input was accepted by the 49K engine. The cold attempt hit the test client’s 20-minute limit on the M1; a cached retry completed in 32 seconds. This is a context/memory check, not evidence of useful long-prompt answer quality or acceptable cold latency.
- Public text artifacts were checked for personal home paths; Windows runtime build metadata is sanitized before packaging.

## Release gates and remaining qualification

The release notes record the final large-prompt smoke test, Mac notarization outcome and archive checksum checks. Physical Windows/Linux execution, end-to-end native desktop coding/tool workflows, cancellation under a real desktop workload and long conversations are not yet fully qualified. The adapters remain previews.

Normal Disconnect/Quit restores only Reflect Go-owned configuration fields and restarts connected desktops without removing account credentials. A crash, forced termination or pulled drive can prevent this; recovery uses the stored restore record on the same computer and same phase. If another app changes an owned field, restoration reports a conflict and preserves that change.

Codex native-account requests retain the normal provider route; local selections never fall back to cloud. Start a fresh conversation when changing providers. Local `/responses/compact` is not implemented. OpenCode's environment or project configuration can override global defaults. Windows Microsoft Store desktop detection is not implemented. The Starter Pack model is intended for simple chat, not these desktop adapters.
