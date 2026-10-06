# Phase 4.5 — final USB test notes

Tested on 2026-10-04, Apple M1 / 16 GiB / macOS 15.7.4, from the actual exFAT Reflect Go stick.

- Full packaged software and both shared-model hashes passed.
- Real 1.5B Metal inference at 2,048 context passed, including authentication, metrics, streaming, generation and shutdown.
- The actual Browser Workstation.command launcher passed authenticated hardware/model discovery. It found both models in /Volumes/Reflect Go/Models and served all three CLI controls.
- Duplicate browser launch, recovery after a forced UI exit, and Quit cleanup passed from the stick.
- The native Mac app opened and detected all three packs, but its model-inventory check timed out, including after Refresh. macOS file-access/loader delays were observed during this automated test. This path remains unqualified; use Browser Workstation.command if it happens on your Mac. No OS privacy policy was changed. The window remains responsive during the delay.
- Windows/Linux/Intel Mac/ChromeOS still require physical tests. The Windows executable is built and packaged, not execution-tested here.

On Windows, open Phase 4.5 and double-click START Reflect Go.exe. This is a portable launcher, not an installer. Reopening it reconnects to the running browser workstation. Finish with Quit workstation.

On Mac, try Reflect Go.app. If the drive check keeps timing out after normal macOS access approval and Refresh, quit it and open Browser Workstation.command. Keep that terminal open; finish with Quit workstation.

The native controller and all three local CLI connections were separately tested. Small-model task accuracy remains limited: Codex and Claude passed the exact file-writing fixture; OpenCode wrote/read the marker but missed its requested newline. More demanding coding repairs were unreliable. See VALIDATION.md and the source evidence for details.

The archive contains the final compiled code and the validation available when it was captured. This companion records subsequent USB deployment tests. The personal USB/dist copies include all three clients; the backup archive excludes Claude binaries, all weights and all runtime/client state. Final documentation updates were byte-checked on the USB after the full software qualification; no binaries changed afterward.
