# Phase 2 — local model provider

Scope: native Apple Silicon provider plus existing-client connectors. Preserve Phase 1 and share model files. No desktop UI, LAN service, daemon, or distributed inference.

Investigated: the pinned native llama.cpp Responses and Messages endpoints, installed Codex CLI 0.150.1 and Claude Code 2.1.250, official provider configuration, the 1.5B chat model, and a 4B instruction model. The existing runtime provides transport and streaming, avoiding a separate proxy for core workflows.

Acceptance:

- Load checksum-pinned shared weights using the portable native runtime.
- Bind only to loopback with a random key; reject unauthenticated requests.
- An unmodified existing client reads a fixture project, makes a real source edit, runs tests, and reports the result. Independently verify the edit and unchanged tests.
- No configured cloud fallback; demonstrate a run with external networking denied.
- Expose startup errors; reject duplicate startup; release the listener at shutdown.
- Preserve chat, original restoration, and model sharing.
- Document measured performance, compatibility limits, and second-Mac testing still needed.

Risks: small models misuse tools; API compatibility is partial; client prompts can exceed local context; client state remains host-owned; exFAT cannot enforce private-file permissions; macOS may block unsigned software. Retain failed trials and judge actual workflows rather than chat-only responses.
