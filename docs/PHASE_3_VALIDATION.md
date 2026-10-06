# Phase 3 — validation, 2026-10-04

Phase 3 adds a real native workstation app around the existing portable launcher and provider. It was built and tested on the Apple M1 / 16 GiB Mac, macOS 15.7.4, including execution directly from the exFAT Reflect Go USB. Second-Mac and unfamiliar-user acceptance remain open.

## Verified behavior

- Native app opened from the local distribution and `/Volumes/Reflect Go/Phase 3/Reflect Go.app`. Process inspection confirmed the USB app, USB llama-server, and sibling USB model path.
- Actual hardware and both shared models appeared. Context changes updated memory estimates; at 32,768 tokens the 4B model was correctly marked over the 8 GiB screening budget, while the 1.5B model remained selectable.
- Local 1.5B and USB 4B models loaded through the UI. Authenticated metrics readiness enabled response/client controls. Model/settings controls were disabled while running.
- The app generated real short responses. Measured resident memory, generated-token count, and speed appeared. Visual review led to a wider sidebar and wrapping explanatory labels.
- The upstream instantaneous throughput gauge returns to zero when idle. The final UI uses cumulative generated tokens divided by generation seconds, so the session average remains visible after a reply. This differs slightly from the runtime's per-request log calculation, which excludes the first generated token in its rate.
- Both client buttons opened their generated Terminal launchers with the chosen disposable project path. Process inspection confirmed the Claude executable started. Terminal UI automation was unavailable; interactive onboarding itself was not automated. Backend connector behavior was independently exercised below.
- Stop & unload completed, cleared resident memory, and released port 8766. Normal app quit and a subsequent app start were exercised. A real occupied-port test produced an error without terminating the other listener. The final error directs the user to Run settings → Advanced.
- The in-app integrity action reported **58 packaged files verified**, including the pinned 4B model. The ad-hoc app signature verified on the USB. Re-signing directly on exFAT encountered AppleDouble metadata; final packaging copied the locally signed app as plain file bytes and verified the result. Model discovery also ignores `._` metadata companions. A final reopen exposed a removable-volume access prompt blocking a saved-settings read on the main thread. Settings loading was moved to a background task so the window and Quit remain responsive while access is pending; the app includes a removable-volume usage description. Updated ad-hoc builds can trigger macOS access approval again. No Developer ID signing or notarization is claimed.
- All **21 native launcher operational tests** passed, including JSON inventory, empty library, exFAT metadata exclusion, context-dependent estimates, damaged files, path escapes, invalid performance settings, existing-output refusal, and archive traversal protections.
- API checks: unauthenticated model access returned 401, authenticated access and metrics returned 200, and a foreign Origin did not receive permissive CORS. The engine listened only on 127.0.0.1.

## Measurements from the USB app

| Observation | Result |
|---|---|
| Model | Qwen3-4B-Instruct-2507 Q4_K_M |
| Context / compute | 16,384 tokens / Metal |
| UI startup measurement | 18.1 seconds, including model check/load and readiness polling |
| Short response | 22 generated tokens / 1.20755 generation seconds |
| UI session average after that response | 18.2 tokens/second |
| Resident engine memory displayed | approximately 4.7 GiB |

These are a short smoke test on this Mac, not a broad speed benchmark. USB caching, other apps, prompt length, context use, and model choice affect results. Resident memory is not complete GPU accounting; the system memory estimate is active + wired + compressed memory, not guaranteed free memory.

## Existing clients, real tool calls

Both tests used the USB-hosted 4B model, existing packaged connectors, and a disposable `Client Project` folder with a README containing an undisclosed test word.

- **Codex CLI 0.150.1:** read the README with `cat README.md` through its own shell tool and correctly reported `PORTABLE`. Exit 0, 17.42 seconds. No file changes requested. The known model-catalog/fallback-metadata warnings remain.
- **Claude Code 2.1.250:** called its Read tool, received the real file contents, and correctly reported `PORTABLE`. Exit 0, 11.90 seconds; README unchanged. External outbound networking was denied at the OS level; a separate external socket probe confirmed `EPERM` (errno 1), while loopback remained permitted.

These checks validate the app's connection to existing client tools. They are not fresh coding-repair acceptance tests and do not supersede Phase 2's recorded failed repair attempts. The 4B model remains limited; Claude Code can display assumed context/cost values unrelated to this local engine. The supplied-key confirmation is explained in the app and connector; its exact interactive wording is based on the user's report and Anthropic's authentication documentation, not an automated screenshot of that screen.

## Preservation and packaging

The only backup archive folder is the source project's top-level `releases/`. Phase 2 source and software were captured before modifications. Identical distribution archives were verified and removed; the unique USB migration archive was copied and verified in the main archive folder before its USB copy was removed. Phase 3 archives exclude models, credentials, state, caches, and AppleDouble metadata.

The USB retains runnable Phase 1 and Phase 2, plus the new Phase 3. Existing model files are reused without per-phase copies. Earlier phase software remains intact; their READMEs and corresponding manifest entries were updated for the collection layout. No prior user engine was signaled by our tests. The pre-existing Phase 2 process exited separately during the session; we did not stop or restart it.

Evidence is in `docs/evidence/phase-3/`: provider checks, raw metrics, sanitized engine log, independent client transcripts/results, client-launch checks, busy-port checks, and workstation observations. The final UI test engines were stopped. No persistent service or inference installation was created.

## Remaining limits

Apple Silicon/macOS only. Another physical Mac, a fresh host account, Gatekeeper download/quarantine flows, and unfamiliar-user usability need real-world validation. Existing clients are required. Abruptly force-killing the GUI can leave the child engine running; ordinary stop/window-close/quit manages shutdown. Current model-quality and client API compatibility limitations from Phase 2 remain.
