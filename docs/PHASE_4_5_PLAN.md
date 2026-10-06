# Phase 4.5 — portable CLI harnesses and simpler launch

Carry pinned native Codex CLI, Claude Code CLI and OpenCode CLI packs beside the existing inference engine. Keep all model weights in the shared Models library, preserve previous phases, and write backup archives only to the source project's releases directory.

The default connector uses the pack on the drive and fails clearly if it is missing or damaged. Host installations are an explicit alternative. Keep client settings, caches and sessions on the drive; do not import host credentials or cloud providers. Preserve each harness's normal project/tool approvals. Project tools and OS facilities are supplied by the host; this does not promise every development toolchain is bundled.

Provide a root Windows executable with one clear start instruction. A second launch authenticates and reopens the existing browser session instead of reporting a confusing duplicate-window error. Retain safe process ownership and provide actionable errors without killing unrelated listeners or bypassing OS policy.

Add all three clients to the Mac and browser interfaces. Validate pinned versions with real local inference and file/tool workflows on this Mac, portable profile isolation, damaged-pack handling and duplicate-launch recovery. Cross-built Windows/Linux packs remain physically unqualified until tested on those devices.

Codex and OpenCode retain upstream open-source notices. Claude Code is proprietary: prepare the user's requested personal drive copy directly from Anthropic's published package, preserve its terms, and exclude that binary from the shareable release archive. Public redistribution is not claimed.
