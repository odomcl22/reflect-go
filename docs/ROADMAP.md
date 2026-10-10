# Reflect Go — roadmap to 1.0

Updated: 2026-10-10. This remains the acceptance roadmap. Core implementation is now in the 0.4.7 engineering preview; see [implementation and validation](PREVIEW_0_4_7_VALIDATION.md) for completed checks and remaining gates. Do not treat implementation as full physical-device or coding-quality qualification.

## Product direction

Reflect Go carries the inference engine, model library, and preconfigured supported CLI clients together. The host supplies compute, memory, operating-system facilities, compatible drivers, and any project-specific build tools. The defining experience is using a familiar client with models on the drive, without configuring the local connection by hand.

Reflect Go manages model availability, loading, hardware adaptation, and client compatibility. Clients manage conversations, projects, tasks, tools, and their normal permissions. Built-in chat is a useful direct interface and setup check; the client workflow remains primary.

This roadmap supersedes earlier proposals to reserve portable clients for a Plus edition. Earlier phase plans and validation reports remain historical records. Phase 4.6 is the current engineering preview; it does not become 1.0 simply by being published.

## Intended service blueprint

| Step | User experience | Reflect Go's responsibility |
| --- | --- | --- |
| Open the application | See the drive's models and available clients. | Detect the host, inspect installed packs, restore drive-relative preferences, and show missing requirements. |
| Start the engine | Make enabled models available; optionally preload a default. | Start a stable local provider without requiring all model weights to be loaded. |
| Open a client | Choose Codex, Claude Code, or OpenCode. Optionally use “Open with project…”. | Launch the portable client with its isolated profile, model catalog, and authenticated local connection. |
| Work in the client | Choose an available model and use the client's normal project/task workflow. | Load the requested model, report loading progress, and serve requests. |
| Change models | Select another supported model in the client. | Finish or explicitly cancel active work, release memory, and load the replacement without misrouting requests. |
| Finish | Close clients, stop the engine, and eject the drive. | Shut down owned processes and flush portable state. |

Project selection in Reflect Go becomes optional. Where a CLI requires a working directory, use a dedicated workspace outside the runtime and model folders; allow an explicit project override. Verify each client's project-switching behavior rather than promise an identical picker in all three.

Available, loading, and loaded are distinct states. Default to one loaded model at a time on memory-constrained hosts. Multiple catalog entries do not mean multiple models resident in RAM. Keep cloud fallback disabled in Reflect Go's local profiles.

## Five core improvements

### 1. Make the portable client workflow the main experience

- Replace the mandatory “select one model, load it, select a project” gate with engine startup and optional model preloading.
- Publish stable model identifiers and per-client catalogs for enabled, compatible models. Do not point every selectable name at the same model.
- Add request routing and controlled on-demand loading behind the local endpoint. Establish a clear busy/queue policy so another client cannot evict a model during generation.
- Preserve client permissions, profile isolation, and drive-relative paths. Keep CLI profiles isolated. Desktop connection adapters must journal only owned fields, preserve unrelated host settings, and restore safely with conflict detection.
- Show only supported model capabilities: tool use, context limits, and reasoning controls that the adapter/runtime can actually honor. Verify mappings per client and model.
- Keep “Open client” and “Open with project…” available as separate paths. Start with the three CLIs; qualify desktop adapters afterward.

**Done when:** a user starts Reflect Go on a supported host, opens each qualified portable CLI without first selecting a project, selects between two compatible models inside clients that support that interaction, and receives responses from the requested model. Switching, cancellation, simultaneous requests, missing packs, and drive relocation behave predictably. Client-specific limitations are visible.

**Implementation:** 0.4.6 added the gateway, catalogs and on-demand loading. 0.4.7 makes a client click start the service automatically, remembers optional projects and per-CLI models/defaults, and adds a native Mac folder picker. DeepSeek Harness and OpenCode Web adapters plus an experimental Claude Desktop profile/model gateway extend the integration work. [Compatibility status](HARNESS_COMPATIBILITY.md) separates implemented adapters from desktop targets. Physical-device, real desktop and representative coding qualification remain required.

### 2. Download and manage models inside the application

- Add a small curated catalog with model purpose, exact format/quantization, download size, estimated memory, source, license, and client compatibility.
- Download directly into the shared `Models/` library, with progress, cancel/resume where supported, space checks, and integrity verification before a model becomes selectable.
- Support importing existing compatible GGUF files. Initially support the formats and architectures accepted by the pinned llama.cpp runtime; do not promise that every GGUF or upstream model works.
- Use plain filters: “Fits this computer,” “Coding,” “Chat,” “Installed,” and download size. Explain why a model is hidden, unavailable, or not recommended.
- Keep one weight copy shared by the application and preserved phases. Clearly warn that removing a shared model affects all consumers.
- Keep model downloads explicit and preserve offline operation after preparation. Partial or failed downloads must not appear as ready models.

**Done when:** a model-free package can acquire a recommended model through the UI, verify it, expose it to an appropriate client, and run it offline after download. Interrupted downloads recover without duplicating weights or exposing incomplete files.

### 3. Recommend models and settings automatically

- Recommend a model, compute backend, and context size based on detected RAM/VRAM, available memory where measurable, model requirements, and intended use.
- Explain recommendations in ordinary language. Separate a lightweight setup test from coding readiness; a model fitting in memory does not establish reliable tool use.
- Use the 8 GB and 16 GB Macs, 24 GB Mac, and 12 GB Windows PC as target profiles, with additional physical qualification on the available Windows/NVIDIA machine. Treat these as test targets, not verified support claims.
- Re-evaluate recommendations on a new host rather than blindly reusing aggressive settings from a larger computer.
- Keep manual overrides under Advanced, with clear memory/context tradeoffs. Do not describe estimates as guarantees.
- Recommend reasoning effort only where supported. Do not substitute temperature or extra output tokens for a genuine reasoning control.

**Done when:** target hosts receive explainable starting settings; those settings complete repeatable chat and applicable client tests without exhausting memory. Unsupported combinations receive an actionable explanation rather than a misleading “ready” badge.

### 4. Make status, failures, and recovery understandable

- Distinguish engine available, model loading, model ready, client starting, client connected, and client exited.
- Report actual download/loading progress when available. Otherwise show elapsed time and the current stage, without invented percentages.
- Explain missing clients, wrong API keys, busy ports, insufficient memory, unsupported models, damaged files, unavailable drivers, and interrupted drive access in plain language.
- Keep failed CLI terminals open and save a readable launch report. Offer a specific recovery action and access to technical details.
- Show measured generation speed, time to first response, load time, and memory where available. Keep unavailable metrics blank and label estimates.
- Make diagnostic export deliberate, redact secrets and personal paths where possible, and let users review the report.

**Done when:** a new user can identify whether failure occurred in the engine, model, or client and follow a recovery action. Physically reproduce and retest the Windows terminal-closes-immediately case; cross-compilation alone does not close it.

### 5. Add simple built-in chat and appearance controls

- Add a Chat tab using the existing Reflect Go sidebar, green accents, typography, and panel design. Keep Workstation and client launchers primary.
- Provide streaming responses, stop generation, new conversation, active-model identity, and honest performance measurements.
- Use the same model library and provider as the CLIs. Chat must not require installing or launching a CLI.
- Start with direct conversation, without a second coding-agent tool system. Clearly separate basic chat from clients that can edit files or execute commands.
- Add Dark, Light, and System appearance with readable controls and consistent behavior in the native Mac and browser interfaces.
- Decide and disclose conversation retention before shipping; if saved, use portable state with an explicit deletion control. Do not silently reuse preview/demo content as real model output.

**Done when:** the starter model completes a real streaming conversation through the app; stop, model changes, engine failure, and all three appearance modes work. The existing client workflow stays easy to find.

**Implementation:** 0.4.6 wires the chat interface into the real local provider and uses the shared interface inside a native Mac window and browser workstation. The earlier design preview remains separate.

## Implementation order and gates

| Milestone | Scope | Exit gate |
| --- | --- | --- |
| A — Reliable client launch | Improvement 4's startup/recovery work; optional project selection; regression qualification of current portable packs. | Repeatable physical Mac/Windows launch, useful errors, clean shutdown, and preserved host settings. |
| B — Model availability | Improvement 1's catalog, capability profiles, stable endpoint, and on-demand model switching. | Two-model selection/routing tests, active-request protection, cancellation, memory release, and per-client capability checks pass. |
| C — Guided setup | Improvements 2 and 3; model filters; shared-library integrity and download recovery. | A fresh model-free package reaches a successful offline session through the UI on qualified hosts. |
| D — Complete everyday experience | Improvement 5; accessibility, theme parity, final status/diagnostic polish. | Real chat and client journeys pass with unfamiliar users; preview/demo state is absent from production. |
| 1.0 qualification | Distribution, security/update behavior, licenses, performance evidence, and supported-platform matrix. | Publish only claims backed by repeatable physical-device evidence and representative client workflows. |

These milestones are execution priorities, not newly assigned release numbers or delivery dates. Prototype UI and packaging work can proceed alongside backend work, but publishing multiple model names must wait for correct routing.

## Performance work across milestones

First establish measurements on the same host, model, quantization, prompt, context, and client. Compare cold load, time to first response, prompt processing, generation speed, peak memory, and successful task completion. Include drive I/O and model-switch costs; avoid repeatedly hashing unchanged multi-gigabyte files without an integrity strategy.

Tune the existing llama.cpp path before adding engines: verify acceleration, GPU offload, supported attention/KV-cache options, context sizing, and prompt-cache behavior using representative workloads. Any cached integrity result must be invalidated appropriately; performance work must not quietly remove verification.

Evaluate MLX and Uzu later as optional Apple Silicon backends with the same workload and compatibility requirements. Research DSpark separately before claiming compatibility or speed gains. New engines must justify their package size, maintenance cost, model-format needs, and client API behavior. None is currently a promised dependency of 1.0.

## Two downloads, one product

| Edition | Planned contents |
| --- | --- |
| Reflect Go | Application, supported inference runtimes, supported portable CLI packs, and preconfigured local profiles; no weights. |
| Reflect Go Starter Pack | The same application and client functionality, plus one small, validated demonstration model. |

No separate Plus feature tier. The small starter model proves setup/basic chat; do not market it as a capable coding model without representative results. Offer a clear path to download a coding model appropriate to the host.

Codex CLI, Claude Code CLI, and OpenCode CLI remain the initial integration targets. The personal copy already includes all three. Resolve a permitted public acquisition/distribution path per client before publishing the download promise. The current shareable archive excludes proprietary Claude binaries; preparation from the vendor may be necessary if public bundling is unavailable. Document any one-time network preparation honestly.

On GitHub, lead with these two download choices, supported/tested platforms, exact launch instructions, sizes, checksums, and a short first-run guide. Keep old phases accessible as historical previews rather than competing main downloads. Use platform-specific assets or component packs as needed, while retaining the same functionality. Do not promise “any computer” or fully bundled project toolchains.

Continue storing generated packages in `dist/`, backup archives only in the project's top-level `releases/`, and shared weights once in `Models/`. Do not put models or personal runtime/client state into source Git or historical software backups. The Starter Pack's explicit distributable model is the intentional exception to model-free download packaging, not permission to copy the whole personal library.

## Deferred scope

Network model sharing, distributed inference, multi-machine orchestration, Reflect Forge/Spark work, autonomous adapter generation, and a new full coding harness remain separate extensions or projects. Desktop client integration is a follow-on after the CLI workflow is qualified; it should reuse the provider/catalog design without delaying the first reliable portable CLI release.

## Reference behavior and existing evidence

- [Ollama Codex CLI integration](https://docs.ollama.com/integrations/codex): model catalog and dedicated launch profile.
- [Ollama desktop integration](https://docs.ollama.com/integrations/chatgpt): client-visible model selection; the desktop workflow is distinct from CLI launch.
- [Official Codex configuration reference](https://learn.chatgpt.com/docs/config-file/config-reference): catalog paths, provider configuration, and model-dependent reasoning controls.
- [Phase 4.5 validation](PHASE_4_5_VALIDATION.md): executed checks, workflow failures, packaging limits, and outstanding physical-device qualification.
- [North Star](NORTH_STAR.md): platform commitments and historical scope.

Reference integrations inform the design; they do not establish that Reflect Go or every bundled client version already implements those features.
