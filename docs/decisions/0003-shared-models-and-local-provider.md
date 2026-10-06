# Shared models and a native local provider

## Problem

Preserve the successful USB Phase 1 demonstration, avoid copying weights per phase, and let an existing client use its own tools and agent loop with local inference.

## Alternatives

Self-contained phase copies duplicate weights. Filesystem links are unsuitable for the tested exFAT target. Installing Ollama/LM Studio or a Python proxy adds dependencies. Cloning a coding agent or building a UI does not help prove this phase. The already pinned native runtime supplies the required API endpoints.

## Decision

Archive original source and binaries without weights. Working phases use an explicit sibling `../Models`; the launcher accepts only that layout or internal `models`, with bounded resolved paths. Logical `models/<filename>` integrity entries resolve through the configured library.

Use native llama-server, loopback binding, random key, foreground lifetime, no web UI, and no native file/shell agent. A process-held file lock rejects duplicate starts from the same folder; occupied ports fail. Runtime environment and writable state remain on the drive. Bounded GGUF metadata supplies full FP16 KV estimates, with headroom, so 16K context can be screened against the model architecture.

Connectors configure only their launched process. Codex uses a custom Responses provider, short general coding instructions, the workspace sandbox, and a one-task exec run. Claude Code uses its Messages gateway, built-in tools, and short recovery guidance; repeat tests still showed failures, so its connector is experimental. Clients own tool execution. No cloud fallback is supplied. Desktop clients are outside the tested scope.

Retain Qwen2.5-1.5B for fast chat; evaluate Qwen3-4B-Instruct-2507 Q4_K_M for agent work. Immutable revisions, hashes, and notices travel with the software.

## Tradeoffs

Phase folders must travel with sibling Models. API support is partial: exploratory Codex logs warn about skipped namespace tools and missing model catalog metadata, while basic shell tools still execute. Judge these by actual tests. One inference slot fits the memory budget; clients should run sequentially. Small-model reliability remains limited. Clients may retain host logs. Anyone able to read exFAT storage can read the provider key.

## Sources

- [Pinned llama.cpp server](https://github.com/ggml-org/llama.cpp/tree/7fe450e19305b828c199d602c23a8337aaa1f03b/tools/server)
- [Codex advanced configuration](https://learn.chatgpt.com/docs/config-file/config-advanced)
- [Codex configuration reference](https://learn.chatgpt.com/docs/config-file/config-reference)
- [Claude Code gateways](https://code.claude.com/docs/en/llm-gateway)
- [Original Qwen model](https://huggingface.co/Qwen/Qwen3-4B-Instruct-2507)
- [Quantized model provenance](https://huggingface.co/lmstudio-community/Qwen3-4B-Instruct-2507-GGUF/tree/4edb920b6f14e3b9284d4502a6485103d72cde05)
