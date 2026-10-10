# Reflect Go 0.4.8 validation

Checked October 10, 2026 on an Apple M1 Mac with 16 GiB RAM. Availability, a compiled artifact, a protocol test, and a qualified desktop workflow are different levels of evidence.

## Follow-up desktop testing

Live testing found a startup bug in the published 0.4.8 ZIPs: desktop launch requests 49,152 context tokens, but the saved-provider connection validator rejected anything above 32,768. The source now accepts the same supported range as the engine. Regression tests exercise authenticated readiness through the saved provider record, including invalid-context rejection.

These results use a locally patched Mac backend and Qwen3 4B Instruct 2507 Q4_K_M on llama.cpp/Metal. They do **not** qualify the original ZIPs or a complete coding workflow.

| Application | Observed result | Remaining qualification |
| --- | --- | --- |
| Claude Desktop 2.31226.1 | The native window showed the Reflect Go connection and returned the requested local reply. Its first request contained 33,820 prompt tokens and took about 492 seconds. | Cold latency is unsuitable for a quick-start experience on this M1; tool use, cancellation and longer conversations remain unqualified. Native Claude model names are aliases mapped to the offered GGUF models. |
| Codex in ChatGPT Desktop 26.1002.52244 | The installed desktop app-server discovered the actual offered model and completed a local turn through the private Responses router in about 14 seconds, using an isolated profile. | The native model picker, live desktop restart and end-to-end desktop tool workflow still need a separate test. The desktop hosting the test controller was not restarted. |
| OpenCode Desktop 1.18.35 | The Reflect Go model appeared in the picker, and a real local answer appeared in the native window after 57 seconds without a refresh or restart. | The older installed 1.18.10 produced a reply but stayed on Thinking until restarted. That live-display failure did not recur in the fresh 1.18.35 test. Coding/tool workflows remain unqualified. |
| DeepSeek Harness 0.1.1-rc.2 | Its browser interface returned a local reply in about 55 seconds using an isolated profile. A second, fresh launch from the native Mac app selected the project and model automatically and returned a reply in 51 seconds. | Coding/tool workflows and physical Windows/Linux launches remain unqualified. |

OpenCode's renderer logged event-stream failures, including on the newer version that completed the visible reply. Similar symptoms are reported in upstream issues [41986](https://github.com/anomalyco/opencode/issues/41986) and [46733](https://github.com/anomalyco/opencode/issues/46733); that is a lead, not proof of the exact cause on this Mac.

Additional source fixes address Claude reconnecting after it repopulates an inactive profile with defaults, DeepSeek finding an installed Node.js when launched from Finder, and registering the selected DeepSeek workspace before opening its browser. The footer now displays the running backend version.

Live Disconnect restored both Claude and OpenCode without signing out. A subsequent Claude reconnect succeeded. Quitting Reflect Go then restored all 12 journaled Claude fields to their previous values, removed the restore record, and closed the owned local services. A final Quit also restored all three journaled OpenCode fields after the 1.18.35 test and stopped the fresh DeepSeek instance. The existing Ollama-launched DeepSeek instance was left separate.

The full Go suite passes, and the changed backend compiles for macOS, Windows and Linux on x64 and ARM64. Compilation is not physical-device qualification. No updated public archives have been published from this follow-up; desktop release qualification remains open.

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
