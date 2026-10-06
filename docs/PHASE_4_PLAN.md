# Phase 4 — cross-platform adaptive runtime

The same drive carries native launchers and pinned runtime packs for macOS, Windows and Linux. Models remain in one sibling Models directory. Earlier phases and their archives are preserved.

Architecture:

- A standard-library Go launcher is cross-compiled for Mac/Windows/Linux on ARM64 and x86-64. Destination hosts need no Go, Python, Node or inference-stack installation.
- Use the existing llama.cpp b11146 model/protocol baseline. Carry Metal/CPU packs for Macs, CPU packs for Windows/Linux, and Vulkan packs for Windows x64 and Linux x64/ARM64. Probe real runtime device availability. Fall back explicitly to CPU if no usable GPU pack/driver is available. CUDA-specific packages are a future optional optimization; NVIDIA cards can use a working Vulkan driver in this build.
- Retain the native Mac workstation, integrating it with the new launcher. Provide an embedded, loopback-only browser workstation for Windows/Linux and as an optional Mac interface. It uses the host browser, contains no remote assets, and clearly offers Stop and Quit; closing a browser tab alone does not stop inference.
- Keep authenticated inference on loopback, bounded startup/shutdown, process ownership, drive-local state, conservative memory screening, GGUF checks, manifest verification and deliberate model selection. Never silently switch models or send requests to cloud inference.
- Move host-client connection setup into native commands for the existing Codex and Claude CLI adapters. Portable harness packs and the full three-client acceptance matrix remain Phase 4.5.

Qualification distinguishes built packages, executed tests and physical-device acceptance. This Mac can validate native Apple Silicon execution; cross-compilation is not proof that a Windows/Linux device works. Add a portable qualification command/report for additional hosts. Do not mark the full physical matrix complete without evidence.

Linux qualification begins with the glibc floor required by the actual packaged artifacts; audit dependencies rather than assuming every distribution works. ChromeOS is conditional through an enabled Linux environment, correct architecture, enough memory, and executable shared storage. No security bypass or OS replacement. If external storage cannot execute binaries, report that limitation instead of silently copying onto the host.
