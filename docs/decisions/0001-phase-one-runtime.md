# Decision 0001: native launcher and bundled llama.cpp process

Status: selected for Phase 1; revisit with measured evidence. Research date: 2026-10-03.

## Problem

Carry local inference software and models between compatible Macs without installing an inference environment on each host. Minimize packaging work while retaining a route to other runtimes, platforms, and clients.

## Alternatives considered

| Option | Evidence and implications |
| --- | --- |
| llama.cpp | C/C++ engine, official standalone macOS ARM64 archives, GGUF models, Metal/Accelerate and CPU execution, MIT license. Offers a direct CLI plus a server for possible later work. Best fit for the smallest real portable bundle. |
| MLX-LM | Strong Apple Silicon focus and many supported models, MIT license. The standard CLI is a Python package, so a self-contained distribution needs an interpreter and dependencies. No performance disadvantage is assumed. |
| MLX Swift LM | MIT Swift package; avoids a Python interpreter and is a credible later native backend. Integrating model loading/tokenizers and assembling Metal resources is additional work compared with an existing prebuilt CLI. |
| Ollama | MIT engine with useful model management. Official macOS workflow centers on an app, CLI integration, a model store, and server lifecycle. A portable wrapper is possible but duplicates more policy and state handling than this phase needs. |
| MLC-LLM | Apache-2.0 project with several device targets. Documented CLI consumes compiled model folders and optional model libraries, adding model-specific packaging work. |
| New Apple-specific runtimes (including baseRT) | Worth measuring later. A performance claim alone does not establish model coverage, packaging robustness, or compatibility for this proof. |

## Decision

Use llama.cpp stable v0.5.0's referenced binary build b11146, not a moving latest URL at assembly time. Verify the release asset SHA-256. Use official Qwen2.5-1.5B-Instruct Q4_K_M as a small Apache-2.0 evaluation model, pinned by repository revision and LFS SHA-256. It is a test model, not a product-wide model choice.

The first measured Qwen3-0.6B test spent its 128-token budget reasoning rather than completing a summary. The direct completion path in b11146 does not propagate the reasoning switch into all template applications. A model-specific `/no_think` prompt produced a correct summary, but requiring that instruction makes the default interaction less useful. Select the non-reasoning instruction model for the distributed example; do not hide or claim generic reasoning control.

Implement the launcher in Swift using only macOS frameworks. Compile once on the assembly machine; never compile on the destination. Use `execve` for the inference process so terminal signals and exit status do not need a daemon supervisor. Keep runtime-specific arguments in one function. Ship no server executable in the Phase 1 product.

Source inspection found that this release's `llama-cli` starts an internal server. Use `llama-completion` in conversation mode instead: direct inference, a foreground process, no HTTP listener, and EOF/Ctrl-C lifecycle. Bundle only its recursive library dependencies. The inspected runtime deployment target is macOS 13.3; newer OS features may still require actual validation on older hosts.

Default to a 2,048-token context with conservative batch sizes, bounded reply length, and a model-size/memory screen. Expose an explicit CPU option. Show estimates as estimates; do not claim universal fit or optimal speed.

## Tradeoffs

- Phase 1 is macOS ARM64 only. Swift/macOS hardware detection is platform-specific; the on-drive layout and process boundary remain reusable.
- Terminal interaction is sufficient for this proof. No dashboard or client compatibility is claimed.
- Model compatibility follows the pinned runtime. Single-file text GGUF models only; unsupported architectures must fail clearly.
- No HTTP listener reduces lifecycle and security scope. Provider protocols and agent tool calling belong to Phase 2.
- An unsigned engineering bundle cannot promise frictionless Gatekeeper behavior. Signing/notarization is a release concern before broad distribution.
- No benchmark across alternatives has been run; the selection is for packaging simplicity, not a claim that llama.cpp is fastest.

## Primary sources

- [llama.cpp overview, backends and license](https://github.com/ggml-org/llama.cpp)
- [Stable release v0.5.0](https://github.com/ggml-org/llama.cpp/releases/tag/v0.5.0) and its `nightly-tag.txt` asset pointing to b11146
- [Pinned CLI documentation](https://github.com/ggml-org/llama.cpp/blob/v0.5.0/tools/cli/README.md), [CLI startup implementation](https://github.com/ggml-org/llama.cpp/blob/b11146/tools/cli/cli-context.cpp), and [direct completion implementation](https://github.com/ggml-org/llama.cpp/blob/b11146/tools/completion/completion.cpp)
- [MLX-LM](https://github.com/ml-explore/mlx-lm)
- [MLX Swift LM](https://github.com/ml-explore/mlx-swift-lm)
- [Ollama macOS documentation](https://docs.ollama.com/macos) and [license](https://github.com/ollama/ollama/blob/main/LICENSE)
- [MLC CLI](https://llm.mlc.ai/docs/deploy/cli.html) and [license](https://github.com/mlc-ai/mlc-llm/blob/main/LICENSE)
- [baseRT](https://github.com/basecompute/baseRT)
- [Qwen2.5 model card and license](https://huggingface.co/Qwen/Qwen2.5-1.5B-Instruct-GGUF) and [initial Qwen3 test model](https://huggingface.co/Qwen/Qwen3-0.6B-GGUF)
