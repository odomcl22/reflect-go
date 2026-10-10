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

The [Codex Desktop test handoff](CODEX_DESKTOP_TEST_HANDOFF.md) records the exact native test steps, existing implementation, and isolated app-server diagnostic for an independent tester.

### Native Codex Desktop test from Claude Code, 2026-10-10

Controller: Claude Code, outside the target app. Package: local `dist/Reflect Go/Phase 4.8` (`0.4.8-preview`, backend containing `196b7be` plus the fixes below). Model: Qwen3-4B-Instruct-2507-Q4_K_M only, 49,152 context, Metal, Apple M1 16 GiB. Target: `/Applications/ChatGPT.app` (`com.openai.codex`). It started at 26.1002.52244 and installed a pending Sparkle update to **26.1007.21159** during the first restart, so the connection results below are for 26.1007.21159.

**Connect: three failures found, fixed and retested.**

| Attempt | Observed | Cause | Fix |
| --- | --- | --- | --- |
| 1 | `ChatGPT is still open; its work was not force-closed`. Nothing applied. | ChatGPT shows its own *Quit ChatGPT? Scheduled tasks won't run while ChatGPT is closed* confirmation when scheduled tasks exist. Reflect Go waited only 15 seconds for a graceful quit. | Quit wait raised; the error now says to answer the app's quit confirmation. |
| 2 | `Load failed`. ChatGPT quit after the confirmation, but nothing was applied and it stayed closed. | The quit was confirmed about 70 seconds after the request, past WebKit's 60-second request limit in the native window. The dropped request cancelled the server-side change. | Quit wait set to 45 seconds, below the web view limit, so the person gets a clear error instead. |
| 3 | `ChatGPT is still open; if it asked to confirm quitting…` although ChatGPT had already exited. | The running check used `pgrep -f`, which matches any process whose command line merely mentions `ChatGPT.app/Contents/MacOS/ChatGPT`. Here that was the tester's own monitor loop; a log viewer or editor would do the same. | macOS now asks by bundle identifier (`application id … is running`), the same identity used to quit. |
| 4 | **Passed.** A decoy process mentioning the path was deliberately kept running. The quit confirmation was answered, ChatGPT restarted, the restore journal was created, and Reflect Go reported the desktop open with local models. | — | — |

Regression tests added: `TestDesktopQuitAllowsTimeForAppConfirmation` (the wait is ≥30 s and <60 s) and `TestDesktopRunningIgnoresProcessesMentioningAppPath` (macOS, with a decoy process). Full Go suite and `go vet` pass for macOS, Windows and Linux. The Phase 4.8 Mac launchers were rebuilt, Developer ID signed (`reflectgo-desktop-fix`), and their manifest entries updated. Integrity check: 425 files verified. The prior binaries are kept privately outside Git.

**Profile changes while connected.** Semantically (parsed TOML), only the journaled keys changed: `model` (`gpt-6.1-sol` → the `reflect-…` ID), `model_catalog_json` and `openai_base_url` (private loopback URL, redacted). `reflect-go-models.json` was created. `auth.json` was byte-identical throughout. ChatGPT's update separately changed its own version strings under `mcp_servers.node_repl.env`. Reflect Go's TOML writer re-serializes the whole file (key order, quote style); values are preserved, and the original file had no comments. Note that formatting churn as cosmetic.

**Native picker.** After the restart, a new Codex task's model picker displayed **Reflect Go · Qwen3-4B-Instruct-2507-Q4…** as the selection.

**Disconnect: passed.** *Restore previous settings* asked to restart ChatGPT. Its quit confirmation was answered, and it reopened signed in with existing chats. The model was back to `gpt-6.1-sol`, the catalog and base-URL keys were removed, `reflect-go-models.json` was removed, the restore journal was removed, `auth.json` was unchanged, and the parsed config matched the original apart from ChatGPT's own update.

**Message and cancellation test (resumed 19:45, same session, with full-screen control approved).** Reflect Go reconnected Codex Desktop after the earlier Disconnect: ChatGPT's quit confirmation was answered, it restarted, and the journal was created. A new project, *Reflect Go Codex Test*, was created in ChatGPT on an empty disposable folder. The picker was explicitly set to **Reflect Go · Qwen3-4B-Instruct-2507-Q4…**, listed above the cloud models. Every reply was read in the native conversation and correlated with the phase's `workstation-engine.log`.

| Step | Prompt | Visible reply | Engine evidence | Time |
| --- | --- | --- | --- | --- |
| 6–7 cold | `Connection check: reply exactly CODEX LOCAL OK. Do not use tools, run commands, or read or change files.` | `CODEX LOCAL OK` | 8,058 prompt tokens in 66.1 s (122 tok/s), 5 generated; progress advanced steadily | ≈70 s to visible reply |
| 8 warm | `Reply exactly CODEX SECOND OK. Do not use tools.` | `CODEX SECOND OK` | 17 new prompt tokens (prefix cache reused), 5 generated, 1.8 s | ≈2 s engine |
| 9 cancel | `Count from 1 to 400, writing each number as a word on its own line. Do not use tools.` | Streaming stopped at "twenty-six"; UI showed *You stopped after…* | `stop: cancel task, id_task = 191`, slot released | — |
| 9 after stop | `Reply exactly CODEX AFTER STOP OK. Do not use tools.` | `CODEX AFTER STOP OK` | completed normally | a few seconds |
| 11 fresh task | `Reply exactly CODEX FRESH OK. Do not use tools.` | `CODEX FRESH OK` | 1,823 new prompt tokens, 6 generated, 19.0 s | ≈19 s |

There were no retries, stuck UI, refreshes or engine errors. Codex also sends small side requests (conversation titles and suggestions, 260–310 prompt tokens) to the selected local model. The disposable folder was still empty afterwards.

**Quit restoration: passed.** *Stop and quit* restored the profile, ChatGPT's quit confirmation was answered, and ChatGPT reopened signed in on GPT-6.1 Sol. The restore journal and `reflect-go-models.json` were removed, and all Reflect Go processes and the engine exited. `model`, `openai_base_url` and `model_catalog_json` match the original, and `auth.json` is unchanged.

**Residual changes made by ChatGPT, not Reflect Go:**
- Accepting ChatGPT's *Trust this folder?* prompt for the disposable folder recorded `trust_level = "trusted"` for the **containing git repository** (`…/Reflect_On_The_Go`), because the folder sat inside it under `.build/`.
- The *Reflect Go Codex Test* project and its two conversations remain in ChatGPT's sidebar.

Future testers should use a disposable folder **outside any git repository**.

**Not run:** the optional file-tool smoke test (`reflect-go-smoke.txt`). It is not qualified, and this account's profile runs without approvals. Saved sanitized screenshots and a redacted engine log are kept locally outside Git under `.build/desktop-qa-claude-20261010/`.

Result: the native Codex Desktop message workflow passes on ChatGPT 26.1007.21159: connect, picker, visible reply, warm reply, cancellation, reconnect, Disconnect and Quit restoration. Cold first-turn latency is about 70 s on this M1 because Codex sends about 8K prompt tokens. Coding/tool workflows remain unqualified.

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
