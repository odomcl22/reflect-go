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

## Native coding/tool test, 2026-10-10

The native message-only results above do not qualify coding. This follow-up uses one disposable directory per harness under `/private/tmp`, outside every Git repository. Each starts with the same seven-line Python fixture: `add` incorrectly subtracts, two assertions check addition, and successful execution prints `LOCAL TOOL OK`. The harness must read the file, change only subtraction to addition, and actually run `python3 smoke.py`; the controller separately checks the exact disk contents and runs the assertions again. Model and package: Qwen3 4B Instruct 2507 Q4_K_M, 49,152 context, Metal, local Phase 4.8 backend containing `196b7be` and `b7bffa5`. This is a functional check on the running M1 Mac, not a controlled performance benchmark.

| Harness | File read/edit/command result | Qualification |
| --- | --- | --- |
| Claude Desktop 2.31226.1 | No file read, edit or command completed. The fixture remained byte-identical to its seed. | **Not qualified within the ten-minute first-tool budget** in the native Code view. See the details below. |
| OpenCode Desktop 1.18.35 | Actual `read`, `edit` and `bash` tool events completed. The native window displayed the one-line diff, `LOCAL TOOL OK`, and a final reply reporting exit code 0. Independent file and execution checks passed. | **Assisted tool pass:** one busy-response retry and one explicit file-path correction were needed. This does not qualify an unattended first-launch workflow. |
| DeepSeek Harness 0.1.1-rc.2 | Actual `read`, `edit` and `bash` events completed in the browser. The successful command returned `LOCAL TOOL OK`; independent exact-file and execution checks passed with exit 0. | **Tool pass after a busy-response retry:** the model also corrected two malformed working-directory arguments itself. The completed retry took 4m 34s. |
| Codex Desktop (ChatGPT 26.1007.21159) | Tested by Claude Code. Actual native `Read smoke.py`, an `echo … > smoke.py` edit and a `python3 smoke.py` shell event completed, and the native Shell event showed `LOCAL TOOL OK`, Success. Independent exact-file and execution checks passed. | **Tool pass, with model mistakes:** two `read_mcp_resource` calls failed (`unknown MCP server 'local'`) before the model switched to the shell. There were no 409s. 7m 55s on a loaded Mac. See below. |

**Claude:** the general task view was stopped after approximately five minutes of prompt processing without reaching tools. A second attempt used the native Code view, the selected disposable folder and Manual approvals. Its session trace recorded seven HTTP 409 busy retries before the main request began. After ten minutes from submission, no tool had executed; the engine was only about 37% through the roughly 47K-token prompt. The native accessibility state said the response had finished while inference remained active. Normal disconnect prompted that one session was still working; quitting that test cancelled engine task 79, which released after processing 18,710 prompt tokens. This establishes an incomplete test and unacceptable first-tool latency for this run, not that the model can never use Claude's tools.

**OpenCode:** the initial main request received HTTP 409 while its 620-token conversation-title request ran. After an explicit retry, the first coding request processed 7,877 prompt tokens and completed its first tool call in about 233 seconds. The model supplied the parent directory's `smoke.py` instead of the selected project's file. That outside-project read was denied. The controller then supplied the exact permitted path; OpenCode read it, changed only `return a - b` to `return a + b`, and executed `cd <disposable-opencode-folder> && python3 smoke.py`. Its stored command event records `LOCAL TOOL OK` and exit 0, matching the native UI and the controller's independent check. The corrected attempt's native completion showed 1m 13s. No cloud model was selected and no controller-side edit substituted for the model's edit.

**DeepSeek:** its first main request received HTTP 409 while a 256-token title request ran. After one explicit retry, the model read and correctly edited the exact supplied file. Two shell attempts failed with `spawn sandbox-exec ENOENT`; inspection of their tool payloads showed nonexistent, misspelled working directories (and an invalid command in the first attempt). `/usr/bin/sandbox-exec` existed and the owned process's PATH included `/usr/bin`. The model corrected its arguments without another controller prompt and executed `python3 <disposable-deepseek-folder>/smoke.py` in the correct workspace. The browser's completed tool event showed `LOCAL TOOL OK`; the harness reports nonzero shell exits explicitly, and none was reported. The controller then verified the exact one-line edit and independently obtained exit 0 with the same output. Workspace Write and normal sandboxing remained enabled. The UI recorded 129 s to first output and 4m 34s for the completed retry. This is a recovered tool workflow, not a clean first-attempt pass.

**Restoration and cleanup:** Disconnect restored all 12 journaled Claude fields and all three OpenCode fields to their saved values, removed both restore records, and required no sign-out. Normal Quit then stopped Reflect Go, its model gateway/runtime and its owned DeepSeek process; all three test ports stopped accepting connections, and no Phase 4.8 backend processes remained. Codex's config and sign-in file were byte-identical to this test's baseline; its desktop was not restarted. The test leaves disposable fixtures and native test conversations for inspection; Claude also recorded trust for its temporary folder and used Manual approvals for that test. Raw screenshots, this test's tool events and engine logs are retained privately under `.build/tool-qa-20261010/`, outside Git.

**Codex Desktop (Claude Code controller, 22:57–23:15).**

- **Setup.** Fixture `/private/tmp/reflectgo-codex-tool.V92Tam/smoke.py` (outside every Git repository; seed SHA-256 `02acab0a…`). ChatGPT was idle before its restart, and its normal *Quit ChatGPT?* confirmation was answered. The new native task ran in project *Reflect Go Tool Test* on that folder. The picker showed **Reflect Go · Qwen3-4B-Instruct-2507-Q…** ✓ and the exact handoff prompt was sent. The account's normal approval controls were left unchanged: the profile already runs without prompts, so no approvals appeared.
- **Native events, in order.**
  1. `Read MCP resource` ×2, both failing with `resources/read failed: unknown MCP server 'local'` (also in Codex's own log as `read_mcp_resource` tool errors).
  2. `Read smoke.py`.
  3. Shell `echo 'def add(a, b): …' > smoke.py`, Success, no output.
  4. Shell `python3 /private/tmp/reflectgo-codex-tool.V92Tam/smoke.py` → `LOCAL TOOL OK`, Success.

  The final reply reported `LOCAL TOOL OK` and exit code 0.
- **Independent check.** The file differs from the seed only by `return a - b` → `return a + b` (still 140 bytes; both assertions unchanged; SHA-256 `9625b49b…`). The controller's own `python3 smoke.py` printed `LOCAL TOOL OK`, exit 0. No other files were created.
- **Engine correlation.** Seven fresh llama.cpp tasks:
  - First turn: 7,929 prompt tokens in 168 s, then 74 generated.
  - Six tool round trips and a follow-up: 68–263 new prompt tokens each, with the prefix cache reused.

  Generation ran at 1.7–2.0 tok/s and the first-turn prompt at 47 tok/s, about a third of the earlier message test. Other apps were loading the Mac (a long-running `node` process near 77% CPU and the Claude desktop app). Codex's log shows no 409 or busy response: Codex Desktop sent its requests sequentially in this run. The WebSocket attempt was refused with 426 and fell back to HTTP, as designed.
- **Namespace investigation.** Each local request carried about 21 Responses tools of type `namespace` plus one `web_search`, which llama.cpp drops; there were 147 warnings in this run. A request captured from the installed Codex CLI 0.162.0-alpha.17.2, using a throwaway profile and a local capture server, shows that the core coding tools are plain `function` tools and reach the model: `exec_command`, `write_stdin`, `view_image`, `request_user_input` and the goal tools. The `namespace` entries are tool groups; the captured one is `multi_agent_v1` (spawn/send/wait/resume/close agent). The desktop profile adds plugin and MCP servers, consistent with its ~21 groups. The native run confirms the file and shell tools worked without conversion. Namespaced plugin, MCP and multi-agent tools, and cloud web search, remain **unavailable to local models**. No namespace conversion was added: flattening them would add many tool schemas to an 8K-token prompt for a 4B model and needs proof that Codex routes flattened calls back to the right namespace. Record this as a documented limitation.
- **Restoration.**
  - **Disconnect:** restored `model` (`gpt-6.1-sol`), removed `openai_base_url`, `model_catalog_json`, `reflect-go-models.json` and the restore journal, and left `auth.json` byte-identical.
  - **Reconnect, then Quit while connected:** the same restoration; ChatGPT reopened signed in, no Reflect Go processes remained, and port 8766 was closed.

  No trust entry was added to `config.toml` for the `/private/tmp` folder. ChatGPT updated itself to internal build 26.1007.30912 during the run and rewrote only its own version strings. The *Reflect Go Tool Test* project and conversation remain in ChatGPT for inspection.

**Overlapping-request fix.** The gateway rejected any request that arrived while another was running (`lease.TryLock()` → immediate 409). With the real Qwen3 4B model on the unchanged Phase 4.8 gateway, a short "title" request sent 1 s into a main request got **409 in 0.0 s**. The gateway now queues: a request waits for the single inference lease. It gives up if its client cancels or the gateway shuts down, and returns 409 only after a 10-minute wait (`gatewayQueueWait`, the same limit as model loading). The lease still prevents a model swap mid-stream. On the patched gateway with the real model, the same overlap returned **200 for both**: the title waited about 33 s and answered *Natural Numbers*. Regression tests: `TestGatewayQueuesOverlappingRequests`, `TestGatewayQueuedRequestLeavesWhenClientCancels`, and the updated busy test (409 only after the queue limit). The race detector and the full suite pass; `go vet` passes for macOS, Windows and Linux. The local Phase 4.8 Mac launchers were rebuilt and signed, and their manifest entries updated (425 files verified). **Public preview binaries were not changed or published.** The native Claude, OpenCode and DeepSeek first-turn tests should be repeated on this build to confirm the 409 retries are gone.

Before this fix, the shared gateway returned HTTP 409 to overlapping requests. This was reproduced in all three harnesses. Real desktop clients make auxiliary requests such as conversation titles, so this is a release issue even with only one person using one harness. The [Codex handoff](CODEX_DESKTOP_TEST_HANDOFF.md#required-codingtool-test-for-claude-code-to-run) now requires the same edit-and-run test and investigation of the previously observed skipped Responses namespace tools. The queueing fix above addresses that cause; public preview binaries have not been changed. **The complete desktop coding experience is not ready for release:**
- Codex Desktop, OpenCode and DeepSeek have demonstrated real native tool execution; OpenCode and DeepSeek needed retries before the queueing fix.
- Claude remains unqualified.
- The three harnesses that hit 409 need a native first-turn retest on the queueing build.
- Cold first-turn latency on this M1 remains minutes long for every desktop harness.

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
