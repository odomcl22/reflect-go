# Phase 4 validation — 0.4.0 engineering preview

Test date: 2026-10-04. Execution host: Apple M1, 16 GiB RAM, macOS 15.7.4.
Phase 4 is packaged for six OS/architecture combinations. **Windows, Linux,
Intel Mac and ChromeOS physical qualification is not complete.** A successful
cross-build or dependency audit is not a successful device test.

## Executed on this Mac

- 15 Go regression tests passed with the race detector. Coverage includes invalid
  flags, bounded GGUF metadata, model eligibility, deliberate model selection,
  path/symlink confinement, atomic settings, device parsing, missing-runtime and
  GPU-budget failures, strict loopback settings, key handling, manifest integrity,
  engine locks, browser Host/Origin/token checks, client environment and redaction.
- Seven C supervisor tests passed: natural exit, EOF, termination, owner crash,
  cancellation during launch, stubborn-child escalation and utility timeout.
- Actual browser-service acceptance passed with real weights: rejected missing
  credentials/foreign origins/hosts/malformed mutations; occupied-port failure
  left the foreign listener alive; 1.5B Metal generation and measured memory;
  duplicate start refused; normal unload; switch to 4B at 16K context; force-killed
  UI owner released its real engine; restart and CPU generation; Quit closed both
  listeners and removed the reconnect token page.
- The native Mac Workstation controller passed corrupt-setting recovery, model
  selection retention when context makes it ineligible, occupied-port messaging,
  real response/metrics/process memory, duplicate-window protection, normal unload,
  4B/16K restart, previous-log retention, and force-quit cleanup.
- The native app was opened and visually inspected: Phase 4.0 branding, two shared
  models, Metal detection, default 4B/16K selection and layout were correct.
- The qualification command passed full software/model integrity, 4B/16K load,
  unauthenticated 401, authenticated model list/metrics, completed streaming,
  nonempty response and released listener after shutdown.

The 1.5B Metal checks measured about **47.6–47.7 tokens/s** with approximately
1.22–1.23 GiB resident process memory. These are short observed runs, not performance
promises. GPU allocations may not all appear in resident memory.

The final browser Quit path also passed five consecutive response-and-cleanup checks.

The browser API and lifecycle were executed directly. Full browser interaction
and visual qualification remain pending: the available test browser blocks the
local-file reconnect page. Native Mac visual inspection was completed separately.
The final USB package was hash-verified and generated a response from its shared
model library. The default 4B/16K configuration was qualified from local storage.

No claim is made that installed-client coding workflows were requalified on all
platforms; portable harnesses and the broader client matrix are Phase 4.5.

## Runtime and package audit

- Six Go native launchers compile: macOS/Linux/Windows, ARM64/x86-64; no destination
  Go, Python or Node dependency. Mac GUI and its C owner process are universal.
- Nine runtime packs; 235 native runtime files inspected for architecture and
  linked dependencies. Audits are shipped as `dependency-audit.json`.
- Mac: upstream b11146 Metal/CPU with bundled dynamic libraries, macOS 13.3+.
- Linux: x86-64 and ARM64 CPU/Vulkan, **glibc 2.38+**. GCC, OpenMP and OpenSSL
  libraries and their notices are included. Ubuntu 24.04 / Debian 13 are the
  intended device-test baselines. Vulkan still needs a compatible host loader/driver.
- Windows: CPU engines built from pinned b11146 source with LLVM-MinGW and static
  C++ support. PE imports require only OS/UCRT libraries, with no MSVC installation
  prerequisite. Generic CPU instructions trade peak speed for compatibility.
  The optional upstream x86-64 Vulkan pack needs existing Vulkan and MSVC libraries;
  Auto falls back to the self-contained CPU pack when its probe fails.
- All upstream downloads are hash-pinned. Models remain only in shared `Models`.
  Software is about 632 MiB. Backup archives omit models, state and credentials.

First-copy Mac library loading initially exceeded the old 15-second probe.
Runtime probes now allow 60 seconds each; native inventory allows 150 seconds for
GPU and CPU probes. Real model startup retains its five-minute deadline. Errors
remain visible. Acceptance also caught and fixed a supervisor exit message that
had hidden the useful occupied-port error.

## Physical qualification matrix

| Target | Built/audited | Actual model execution |
| --- | --- | --- |
| M1 Mac, local storage, Metal and CPU | Yes | Passed |
| M1 Mac, Reflect Go exFAT USB, Metal | Yes | Passed: 1.5B / 2K, integrity, authentication, metrics, streaming, generation, shutdown |
| Intel Mac | Yes | Pending |
| Windows x86-64 CPU/Vulkan | Yes | Pending |
| Windows ARM64 CPU | Yes | Pending |
| Linux x86-64 CPU/Vulkan | Yes | Pending |
| Linux ARM64 CPU/Vulkan | Yes | Pending |
| ChromeOS enabled Linux environment | Conditional Linux package | Pending |

On each target, run `Verify Portability` and retain `state/qualification.json`.
It records model, context, backend, hardware, time and results without keys.
Also check real browser controls, client terminal opening, drive execution and
permissions, fresh-copy startup, unplug workflow, occupied ports and owner crash.
Test GPU and CPU separately. A Linux noexec mount, older glibc, missing driver,
unsupported CPU or managed Chromebook policy can prevent portability.
