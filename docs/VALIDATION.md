# Phase 1 validation — 2026-10-03

Update, 2026-10-04: the user successfully launched the original release from `/Volumes/Reflect Go/PortableAI`, an external exFAT USB, on the same M1 Mac and completed an interactive chat. This supplies physical-drive evidence; a second Mac remains untested. The original source and binaries are preserved in `releases/phase-1-original-2026-10-03.tar.gz` without weights. Phase 2 and the updated shared-model launcher are tracked in `PHASE_2_VALIDATION.md`. Earlier measurements below remain historical results for the original build.

**Result: a working portable-folder implementation, with real inference and relocation tested on one Mac. The complete Phase 1 success condition remains open until a physical external drive and a second compatible Mac are tested.**

## Tested artifact and host

- `dist/PortableAI`, approximately 1.14 GB decimal / 1.06 GiB before test reports.
- Apple M1, 8 CPU threads, 16 GiB unified memory, macOS 15.7.4 (24G503).
- Metal reports Apple M1 with a recommended working set of 10.67 GiB. PortableAI uses an 8 GiB screening budget and estimates 3.35 GiB for the bundled model at 2,048 context tokens.
- llama.cpp v0.5.0 / b11146, direct `llama-completion` command and nine dependent libraries.
- Official Qwen2.5-1.5B-Instruct Q4_K_M, 1,117,320,736 bytes; SHA-256 `6a1a2eb6d15622bf3c96857206351ba97e1af16c30d7a74ee38970e434e9407e`.
- Eleven Mach-O files audited, including the native launcher. All declare macOS 13.3 and ARM64; all dynamic dependencies resolve to bundled libraries or macOS system frameworks/libraries. Minimum deployment target is not a second-host validation.

## Observed results

| Check | Result |
| --- | --- |
| Real Metal summary | Passed; runtime logged **29/29 layers offloaded to GPU**, actual context 2,048 |
| Real CPU generation | Passed; runtime logged **0/29 GPU layers**, actual context 2,048 |
| Useful generated code | A real generated `clamp(value, low, high)` function passed five below/inside/above/boundary inputs in a restricted evaluator |
| Integrity | Runtime, launcher, bundled model, notices, and documentation verified against the bundle manifest; 43 files after adding the destination check script |
| Launcher failure tests | 16 passed: missing/invalid/truncated/split/oversized models, escaping symlink, malformed config, invalid options, missing runtime, corrupt hash, archive traversal, existing-output preservation, and relocated hardware inspection |
| Offline relocation | Entire bundle copied to a temporary path containing spaces and Unicode; real inference passed under macOS `sandbox-exec` with **all network access denied** |
| Host dependencies | Relocation used system-only PATH and an empty HOME. The original empty HOME remained untouched. No Python/compiler/package manager participated in inference |
| Inherited remote settings | Test injected model URL, HF repository, and RPC environment variables. They were not passed through; offline inference still succeeded |
| Interactive interaction | Menu selection, a real response, and Ctrl-D on an empty prompt returned exit code 0 |
| Interruption | SIGINT during generation followed by a second SIGINT exited with code 130 |
| Terminal hangup | SIGHUP terminated the process |
| Background/network lifecycle | `lsof` found no network sockets for the inference PID; `pgrep` found no child process; tested processes exited |
| Context overflow | 730-token prompt with context 512 was refused with exit code 1 and an explicit maximum of 508 input tokens |

Representative observations, **not a controlled cross-backend benchmark**:

| Workload | Generation speed | Maximum process RSS | Wall time |
| --- | ---: | ---: | ---: |
| Metal summary, warm location | 18.97 tokens/s | 1.22 GiB | 3.46 s |
| CPU short Python function | 17.96 tokens/s | 2.07 GiB | 5.57 s |
| Metal summary, fresh relocated folder, network denied | 19.51 tokens/s | 1.23 GiB | 46.37 s |

The short workloads differ, the machine was not isolated for benchmarking, and process RSS is not total host/GPU memory use. The fresh-location startup delay is observed but its cause has not been isolated. Do not advertise warm startup as first-run latency.

The successful summary, with an explicit request to preserve both days, both times, and online availability, was:

> The library will be closed on Friday from 3 pm to Monday at 9 am, but online borrowing services will continue.

The generated function was:

```python
def clamp(value, low, high):
    return max(low, min(value, high))
```

## Quality failures retained

The initial Qwen3-0.6B test spent a 128-token reply budget on reasoning. The current direct completion implementation did not honor the reasoning switch through that template path. A model-specific `/no_think` instruction worked, but the default bundle was changed to the non-reasoning instruction model instead of hiding this limitation.

The Qwen2.5 model's first generic one-sentence summary omitted the exact times. A more explicit prompt retained them. This demonstrates useful bounded inference, not dependable reasoning on arbitrary tasks. No coding-agent loop, tool calling, or external AI-client compatibility has been tested or claimed.

## Evidence and reproduction

Durable compact evidence is in `docs/evidence/`: hardware JSON, integration measurements and outputs, test summary, and context-overflow result. Full development logs, including loading/offload and memory records, are in `.build/evidence/` (excluded from source control). `dist/PortableAI/dependency-audit.json` and `manifest.json` travel with the artifact.

From the source project on the assembly Mac:

```sh
python3 tests/test_bundle.py dist/PortableAI
python3 tests/integration.py dist/PortableAI
```

On a destination Mac, double-click `VerifyPortability.command` inside the copied bundle. It needs no developer tools and saves a local report under `state/portability-check.*`. The script itself was exercised on the development Mac; this does not substitute for the physical tests below.

## Required to close Phase 1

1. Copy the complete folder onto a real writable external APFS drive. Launch from the drive with the original project unavailable. Run the destination check and an interactive chat; confirm actual GPU offload, correct response, clean exit, and successful eject.
2. Move the **same directory and drive** to a second Apple Silicon Mac without an installed inference stack or development tools. Run offline without rebuilding or installing anything. Retain both hardware reports and output logs on the drive, then verify clean shutdown and eject.
3. Repeat on macOS 13.3 if that minimum is to be advertised as tested. Verify normal Gatekeeper/quarantine behavior with a downloaded distribution. Developer ID signing/notarization remains undone.

External drive throughput, exFAT, volume permission policies, another Mac, and another macOS release are untested. No external drive was mounted during this session. No Phase 2–8 functionality was implemented.
