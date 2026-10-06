# Phase 2 validation — 2026-10-04

Host: Apple M1, 16 GiB unified memory, macOS 15.7.4. Native llama.cpp b11146 at pinned source revision `7fe450e19305b828c199d602c23a8337aaa1f03b`. Agent model: Qwen3-4B-Instruct-2507 Q4_K_M, SHA-256 `8cdb57cbb880d313736a9bc4e3d3d2485f145b5e19cf33783746e753e82641fc`. Default provider context: 16,384; estimated memory: 7.40 GiB against an 8 GiB static budget. This is not a free-memory measurement.

## Real client acceptance

The fixture is a broken shell `clamp` function with five boundary/interior tests. The clients must read the files, change the implementation, and run the tests. Independent `sh test.sh` checks and byte comparisons verify the source changed and the test file did not.

| Client and configuration | Result |
|---|---|
| Codex CLI 0.150.1, original 1.5B model | Failed: prose without an edit; independent tests failed. |
| Codex CLI, 4B, default long instructions, interleaved with another client | Tool calls worked, but the trial timed out at 600 seconds before an edit. |
| Claude Code 2.1.250, 4B, bare mode | Passed: Read → Edit → Bash test. The first edit failed; the model recovered using the tool error, then all five tests passed. |
| Packaged Codex connector, 4B, compact general instructions, sole client | Passed: read source/tests, edit source through real tools, run test, report success. Independent tests passed and test.sh was unchanged. |

Codex's normal agent loop, file/shell execution, and sandbox performed the work. Claude Code's built-in tools performed its work. The outer test scripts did not write the solution. The fixture and client-generated source are retained in `tests/fixtures/agent-project` and `docs/evidence/phase-2`.

The Claude trial was interleaved with Codex on one inference slot and took 504 seconds including repeated prefill/cache replacement. It is not a useful standalone speed benchmark. The packaged sole-client Codex run's server requests took about 217 seconds in total; individual generation rates were **6.52–7.25 tokens/second**, with initial prefill 70.46 tokens/second. These are one-host observations, not cross-machine guarantees.

## Provider and launcher checks

- Authenticated model list and metrics: HTTP 200; unauthenticated model list: HTTP 401.
- Listener observed as `127.0.0.1:8765`, not a wildcard address.
- Foreign Origin did not receive a matching or wildcard CORS allow-origin.
- A second start from the same folder failed with the provider-lock message while the first process continued.
- Provider stopped with SIGINT before refreshing the final launcher.
- Final native launcher: 18 operational tests passed, including shared paths, escaping model paths, bad ports, damaged files, oversized model refusal, and archive traversal rejection.
- Updated Phase 1 completed inference with all networking denied and answered that it was running locally. Generation measured 18.09 tokens/second for that short answer.
- Original source/binary archive checksum and inventory verified; no model weights are in the archive.

## Physical USB and network isolation

Both phase manifests passed directly on `/Volumes/Reflect Go`, including their shared models. The provider loaded `/Volumes/Reflect Go/Models/Qwen3-4B-Instruct-2507-Q4_K_M.gguf` using the bundled on-drive runtime. Its first startup at this location took about 42 seconds in the runtime log; that does not isolate the cause of the initial delay. Authentication, CORS, metrics, and loopback binding checks passed there too.

An OS sandbox allowing outbound connections only to localhost was applied to the Claude client. A separate socket probe received errno 1 (EPERM) when attempting a non-loopback connection. The isolated client made real local inference requests, read the fixture, edited it, and ran its tests. **The repair failed**: the small model introduced unsupported `bc` functions and then repeated incorrect string replacements. We stopped that trial after 309 seconds. A fresh run with general recovery guidance also failed and was stopped after 185 seconds. Both transcripts and failed outputs are retained. No successful isolated end-to-end repair is claimed.

An earlier harness attempt used unsupported numeric-address syntax in the macOS sandbox profile and exited before starting the client. The harness was corrected to the supported `localhost` syntax and now requires an actual EPERM socket denial, not just a failed curl request.

Claude's original successful repair proves capability, while these repeat failures show poor reliability with this small model/tool combination. The connector is marked experimental. Codex is the recommended starting client based on its successful packaged-connector test. The first direct-USB Codex repeat also failed to edit the file because the model repeatedly produced ineffective sed commands; we stopped it after 395 seconds. The final Codex profile adds general guidance to prefer straightforward writes for small files and avoid complex shell escaping. With that profile, a fresh USB run **passed in 213.24 seconds**: Codex read both files, wrote a correct implementation, ran the test script, and reported success. Independent `sh test.sh` passed all five tests and a byte comparison confirmed the tests were unchanged. That run also recovered from an invalid process/session tool call. It demonstrates capability, not consistently reliable editing.

The final result satisfies the core Phase 2 proof with the unmodified Codex CLI using the USB-hosted model. A complete successful client repair under the external-network-denied harness remains unproven; isolated local inference and tool use are demonstrated. Claude remains experimental. A second compatible physical Mac remains untested.

## Limits and interpretation

This is a narrow agent integration proof, not a coding benchmark. Codex logs warn about missing model catalog metadata, shortened skill descriptions, and unsupported Responses namespace tools; core shell tools still executed. Desktop applications, additional tools, image input, cloud API parity, long projects, and automatic compaction are not validated. Claude Code reports an assumed context and estimated cost for unknown models; the provider's configured context is 16K and no cloud inference fallback is configured.

The original Phase 1 is user-confirmed on the exFAT USB and this M1. A second physical Mac has not been tested. Original empty-message continuation behavior remains documented; the exact keystroke sequence preceding the user's exam-question output was not reproduced. Local system instructions do not guarantee truthful model self-description.

## Final shutdown and Phase 1 regression

The USB provider was stopped with SIGINT and TCP port 8765 was confirmed released. The updated Phase 1 then passed its built-in VerifyPortability check directly on the USB with all process networking denied. Its summary retained Friday 3 PM, Monday 9 AM, and continuing online borrowing, with awkward punctuation. The on-drive report is `Phase 1/state/portability-check.TeX9T6`. Both the preserved original and current shared-model working release remain available.
