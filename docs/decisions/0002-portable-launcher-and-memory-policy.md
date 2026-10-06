# Decision 0002: precompiled launcher and conservative memory screening

Status: implemented for Phase 1. Date: 2026-10-03.

## Problem

Detect the Mac's actual CPU/memory/Metal capabilities and choose a local model without requiring another installed runtime, writing settings into the user's home, or introducing a long-running supervisor.

## Alternatives

- A shell-only launcher would avoid compilation but would parse command output for structured hardware data and depend on more host commands.
- Python would be convenient for metadata/configuration, but either violates the no-installed-interpreter constraint or adds an interpreter distribution to maintain.
- Go or Rust could produce a native launcher and offer a future cross-platform core, but accessing Apple's Metal device properties adds bindings or a separate helper before those platforms are in scope.
- Objective-C/C and Swift can directly query macOS/Metal. Swift's Foundation JSON and file APIs reduce manual parsing and memory management for this small program.
- Exact model memory forecasting requires architecture-specific tensor/KV/buffer knowledge or trial loading. File size alone is not a guarantee.

## Decision and rationale

Compile a small Swift executable against system Foundation, Metal, CryptoKit, and Darwin. Use no package dependencies. The builder audits its dynamic dependencies along with the runtime's. macOS 13.3 is the packaged binary floor. Compile once during assembly; destination users only execute the bundle.

Reserve half of physical RAM, capped by Metal's recommended working-set limit when present. Estimate required bytes as 1.3 × model file bytes + 1 GiB + context tokens × 512 KiB. This is an intentionally coarse screen for this bounded-context proof. Label it explicitly and refuse models above the screen. Choose the smallest eligible local model by default; let users select another eligible model. Do not claim this selects the smartest or fastest model.

Keep settings as a plain on-drive JSON file. Resolve files relative to the actual executable, reject model symlinks escaping the bundle, use a clean child environment, and replace the launcher with direct inference using `execve`. `state/` stores app-controlled writable locations. No background service or model-provider abstraction is needed in this phase.

## Tradeoffs

- The launcher is macOS-specific and the assembly Mac needs Xcode tools. A later cross-platform implementation may replace it without changing the model/runtime directory boundary.
- The memory screen can reject models that would fit and cannot prevent contention or all allocation failures. It does not measure currently available memory. Exact fitting is deferred until needed and measurable.
- EOF and signals follow the pinned runtime's terminal behavior. There is no automatic recovery from a killed runtime; failure returns to the user's shell.
- Environment redirection is not a sandbox or a promise of zero host traces. macOS and Terminal can maintain logs/caches/swap. A separate network-denial sandbox is used for acceptance testing.
- Unsigned code and external-volume policies can require normal macOS approval. No automatic security-policy changes are made.

Evidence: `dist/PortableAI/dependency-audit.json`, `docs/VALIDATION.md`, and the source in `Sources/PortableAI.swift`.
