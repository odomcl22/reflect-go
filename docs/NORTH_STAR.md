# Reflect Go — Project North Star

## Current roadmap — 2026-10-09

The [roadmap to 1.0](ROADMAP.md) is the current forward plan. It puts preconfigured
portable CLI clients in the core product, makes project selection optional,
and introduces client-visible model catalogs with on-demand loading. It also
covers in-app model downloads, hardware-aware recommendations, clear status and
recovery, and simple chat with Dark/Light/System appearance.

The planned downloads are **Reflect Go** (software and supported clients, no
weights) and **Reflect Go Starter Pack** (the same functionality plus one small
demonstration model). This supersedes the optional Plus packaging proposal below.
Public client distribution remains subject to a permitted acquisition method.
Phase 4.5 remains preserved. The 0.4.6 development preview implements the core
workflow; [its validation](PREVIEW_0_4_6_VALIDATION.md) distinguishes executed
checks from outstanding roadmap acceptance gates. Earlier scope and phase descriptions are retained below.

## Agreed product scope — 2026-10-04

**Reflect Go** (long name **Reflect on the Go**) is the portable inference engine. Its core product carries model weights, inference runtimes, and configuration on an external drive, adapts to supported host hardware, and connects supported harnesses. The host supplies the operating system, compute, memory, compatible drivers, and permission to execute the application.

The next milestones are:

| Milestone | Scope |
| --- | --- |
| 3.1 | Harden the current workstation: startup, shutdown, recovery, diagnostics, model switching, and first-run usability. |
| 4.0 | Qualify the adaptive runtime on an explicit hardware/OS matrix, covering supported Windows, Mac, and Linux computers. Assess ChromeOS through its Linux development environment as a conditional compatibility target. |
| 4.5 | CLI-first harness compatibility and optional portable harness packs, with all three initial targets: **Codex CLI, OpenCode CLI, and Claude Code CLI**. |
| Reflect Go 1.0 | Release after the agreed core scope passes repeatable acceptance checks on supported physical machines. |

Claude Code is an initial target alongside Codex and OpenCode. Its connection to local models has already been demonstrated in Phases 2 and 3; packaging the CLI on the drive and qualifying its supported workflows remain separate work. Existing validation reports still describe the limits of the tested releases and small models.

### Platform commitments

Phase 4 treats **Windows, macOS, and Linux** as core targets. Qualify specific OS versions, CPU architectures, GPU backends, and removable-filesystem permissions; “Linux” does not mean every distribution. Begin Linux qualification with a defined Ubuntu/Debian baseline and CPU inference, then test acceleration and additional distributions. Include both x86-64 and ARM64 in the target matrix, with support claims only after qualification.

**ChromeOS/Chromebooks** are a conditional target through Google's Linux development environment (Crostini), not a native ChromeOS executable. A compatible device must have that environment enabled and permit access to the drive. Managed-device policy may prohibit it. Verify executable permissions on shared external storage, architecture, memory, loopback access, and available acceleration on actual hardware; do not promise direct execution from every USB mount. If binaries require container-local staging, disclose that exception to the no-install/no-host-copy experience. We will not require disabling ChromeOS security or replacing the OS.

Google documents enabling the environment in its [Chromebook setup guide](https://support.google.com/chromebook/answer/9145439) and describes Linux file sharing in its [developer setup guide](https://developers.google.com/chromeos/app-development/develop/setup). This is a research/qualification target, not a claim that Phase 3.1 runs on Linux or ChromeOS.

### Phase 4 implementation status

The 0.4.0 engineering preview packages native launchers and adaptive runtime packs
for Windows, macOS and Linux on x86-64 and ARM64. Mac keeps its native app;
Windows/Linux use a local browser workstation. Windows CPU inference is built
without a required MSVC installation. GPU paths depend on compatible host drivers.
The qualification command produces per-host evidence. Physical Windows/Linux,
Intel Mac and Chromebook checks remain a release gate, not an inference from
cross-compilation. See [the Phase 4 validation matrix](PHASE_4_VALIDATION.md).

### CLI-first harness packs

Users should be able to choose a supported harness already installed on the host or a prepared harness pack on the drive. Packs share the collection's model library and use Reflect Go's local provider. They do not contain separate copies of the weights or provide their own inference engine. Reflect Go Plus is a possible packaging name, not a separate engine implementation.

Each initial CLI must be evaluated for:

- platform-specific executables and required dependencies;
- drive-relative launch and configuration that survive drive renaming or Windows drive-letter changes;
- isolated settings and documented locations for sessions, caches, temporary data, and credentials;
- real local-model streaming, file reads, edits, checks, follow-up turns, and cancellation;
- clear reporting of unavailable features and no silent cloud fallback;
- controlled updates and recovery without changing an existing host installation;
- a valid acquisition/distribution method, whether bundled redistribution or preparation from the vendor's download, as applicable.

Start with the three CLIs. Assess graphical harness packs after CLI portability is proven; desktop packaging is not required to complete this initial scope. A client being extensible or accepting a local API does not by itself establish portable packaging or compatibility with every model.

### Deferred work

Network sharing, distributed agent work, and distributed model inference remain separate research/product directions. Reflect Go may later connect to them through an optional extension. ReflectForge is an existing independent product, with Reflect for persistent context, Forge for execution, and Spark for the user-facing intelligence; this roadmap does not redefine or restart it.

Automatic detection, known adapters, reversible configuration, and compatibility checks fit the core. Autonomous generation of adapters for unfamiliar harnesses is deferred to an optional development workflow.

The original phase descriptions below preserve the project's background. This agreed scope takes precedence: original Phases 5–8 are deferred research, not requirements for Reflect Go 1.0. Phase numbering describes development milestones, not product release versions.

---

## Original vision and phase descriptions

We are building a portable AI platform that can live on an external drive and turn a compatible computer into a local AI workstation without requiring the user to install a separate inference stack on that computer.

The external drive should contain everything necessary to provide local AI inference except the host computer's compute resources.

The host contributes:

- CPU
- GPU
- RAM / unified memory
- operating system capabilities

The portable platform contributes:

- the software required to run inference
- models
- configuration
- model management
- hardware detection
- compatibility with external AI applications
- eventually distributed compute and orchestration

The long-term user experience should be as close as possible to:

> Plug in the drive → launch PortableAI → choose a model → use local AI.

The user should not need to separately install or configure Ollama, LM Studio, llama.cpp, MLX, Python environments, model runtimes, or similar infrastructure.

---

# Ultimate Outcome

The ultimate product is not simply a portable version of llama.cpp or Ollama.

The goal is a **portable AI execution layer**.

It should eventually allow existing AI applications and agent harnesses to use local models as though they were talking to their normal model provider.

Examples may include:

- Codex
- Codex Desktop
- Claude Code
- Claude Desktop
- IDE agents
- custom applications
- other OpenAI-compatible or Anthropic-compatible clients

PortableAI should sit underneath those applications.

Conceptually:

Existing AI Application
↓
PortableAI
↓
Local Model
↓
Best available host hardware

The AI application should not need to care whether the model is running through llama.cpp, MLX, CUDA, Metal, another runtime, or eventually another machine.

PortableAI should handle that abstraction.

---

# Guiding Principle

Do not design PortableAI around one current application, model, inference engine, operating system, or hardware vendor.

Design toward this principle:

> A portable intelligence layer that adapts itself to the hardware it finds.

However, do not overengineer the first implementation in anticipation of everything that may come later.

Each phase should produce something useful and working before moving forward.

---

# PHASE 1 — PROVE PORTABLE LOCAL INFERENCE

## Objective

Prove that meaningful local LLM inference can be run directly from a portable external drive without requiring a traditional inference installation on the host computer.

The initial target can be Apple Silicon/macOS.

A user should be able to:

1. Put PortableAI on an external drive.
2. Put one or more compatible local models on that drive.
3. Connect the drive to a Mac.
4. Launch PortableAI.
5. Have PortableAI understand the capabilities of that Mac.
6. Select an appropriate model.
7. Run the model using the Mac's available CPU/GPU/unified memory.
8. Interact with the model.
9. Shut PortableAI down cleanly.
10. Disconnect the drive.

The inference environment should travel with the drive.

## Phase 1 success condition

We should be able to take the same PortableAI directory or drive to another compatible Mac and run it without rebuilding or reinstalling the inference environment.

This phase proves the central idea.

Do not build a large UI or distributed system yet.

---

# PHASE 2 — BECOME A LOCAL MODEL PROVIDER

## Objective

Make PortableAI usable by existing applications rather than only by PortableAI itself.

PortableAI should expose local models through common interfaces expected by modern AI clients.

The desired experience is:

Codex / Claude / another application
↓
PortableAI
↓
Local model

The external application should use its own:

- interface
- agent loop
- file tools
- shell tools
- coding workflow
- project management
- conversation interface

PortableAI should provide the model inference underneath it.

This phase should determine the most reliable way to support clients such as Codex and Claude without unnecessarily modifying or cloning those applications.

## Phase 2 success condition

An existing AI client can perform meaningful work using a model hosted by PortableAI rather than its normal cloud model provider.

For example, a coding agent should be able to inspect and modify a test project while its inference is coming from a local model running through PortableAI.

The goal is not merely receiving a chat response.

The application should still be capable of performing its normal agent workflow.

---

# PHASE 3 — PORTABLE AI WORKSTATION

## Objective

Turn PortableAI from an inference utility into a polished portable AI environment.

At this stage the user experience matters.

The product should make it easy to:

- see the computer's available AI hardware
- see available models
- understand which models are appropriate for the current machine
- load and unload models
- see memory usage
- see inference performance
- start or stop the local AI provider
- launch or connect supported AI clients
- configure context and performance settings
- diagnose problems

The user should not need to understand inference runtimes or command-line flags.

PortableAI should make intelligent decisions automatically while still allowing advanced users to override them.

## Phase 3 success condition

Someone familiar with AI applications but unfamiliar with llama.cpp, MLX, CUDA, Metal, or model-server configuration should be able to use the product successfully.

The experience should feel like an application, not a collection of developer tools.

---

# PHASE 4 — CROSS-PLATFORM ADAPTIVE RUNTIME

## Objective

Make the same PortableAI drive useful across different classes of computers.

Examples:

Apple Silicon Mac
Windows machine with NVIDIA GPU
Windows machine without a discrete GPU
Linux workstation

PortableAI should detect what it is running on and determine the best available execution strategy.

For example:

64 GB Apple Silicon machine
→ potentially run a larger model

16 GB Apple Silicon machine
→ choose a smaller model or more conservative configuration

NVIDIA workstation
→ use appropriate GPU acceleration

CPU-only computer
→ fall back appropriately

The user should not have to manually understand which inference backend belongs to which machine.

## Phase 4 success condition

The same PortableAI drive can move between supported computers and intelligently adapt itself.

The hardware changes.

The user experience remains substantially the same.

---

# PHASE 5 — NETWORKED PORTABLE AI

## Objective

Allow computers running PortableAI to cooperate across a trusted local network.

Initially, do not assume that multiple machines should behave like one giant pool of RAM.

Instead, allow each computer to become an available AI node.

Example:

Mac Studio
→ large reasoning model

Windows GPU workstation
→ coding model

MacBook
→ smaller fast model

PortableAI should know:

- what machines are available
- what models they can run
- what resources they have
- whether they are busy
- how quickly they perform
- what capabilities they provide

## Phase 5 success condition

A user can run PortableAI on multiple machines and have one environment recognize and make use of the others.

The machines become cooperating AI resources instead of isolated installations.

---

# PHASE 6 — DISTRIBUTED AI WORK

## Objective

Use multiple machines and models intelligently.

This is primarily about distributing **work**, not necessarily splitting one model across several computers.

Examples:

One model plans.
Another writes code.
Another reviews the code.
Another runs tests.
Another critiques the result.

PortableAI should eventually be capable of deciding where a task should run based on:

- model capability
- hardware capability
- available memory
- current workload
- latency
- specialization
- task requirements

The user should be able to benefit from several machines and several models without manually coordinating every request.

## Phase 6 success condition

Several local models on several computers can collaborate on one larger objective through PortableAI.

---

# PHASE 7 — DISTRIBUTED MODEL INFERENCE

## Objective

Investigate whether one model can efficiently span multiple machines when doing so is actually advantageous.

This may include techniques such as:

- layer distribution
- pipeline parallelism
- tensor parallelism
- remote inference workers
- distributed KV/cache strategies
- other emerging techniques

Do not assume this is always better than running separate models.

Network bandwidth, latency, hardware differences, memory location, and inference-engine capabilities must be considered.

## Phase 7 success condition

PortableAI can determine when distributed inference is beneficial and, where technically practical, use multiple physical machines to run workloads that would otherwise not fit efficiently on one machine.

This phase is exploratory until proven worthwhile.

---

# PHASE 8 — PORTABLE COMPUTE FABRIC

## Long-Term Objective

PortableAI ultimately becomes a hardware-independent local AI compute fabric.

Applications should be able to request intelligence without needing to know:

- which machine runs the model
- where the model is stored
- which inference engine is being used
- which GPU architecture is underneath it
- whether one model or several models are working
- whether computation is local or distributed across trusted nodes

Conceptually:

User / Application
↓
PortableAI
↓
Intelligent scheduler
↓
Available models + available hardware
↓
Best execution strategy

PortableAI becomes the layer that connects AI applications, models, and available computing resources.

---

# PRODUCT PRINCIPLES

Keep these principles throughout development.

## Portability First

The project must genuinely remain portable.

Do not slowly turn it into a conventional installed application unless installation is optional.

## Hardware Adaptive

PortableAI should use the hardware available rather than requiring one specific configuration.

## Model Agnostic

Do not build the product around one model family.

## Runtime Agnostic

Do not unnecessarily bind the entire architecture to one inference engine.

## Client Agnostic

Codex and Claude are important targets, but PortableAI should eventually support other applications.

## Local First

Local execution is the central purpose.

Cloud models may someday be supported as another resource, but they should not become a requirement.

## Progressive Complexity

Do not solve Phase 7 while building Phase 1.

Build the smallest system that completely proves each phase.

## Preserve the User's Machine

Avoid unnecessary system-level changes.

Do not require administrator privileges unless absolutely unavoidable.

Do not install persistent services unless the user explicitly chooses to.

Changes made to the host should be minimal, transparent, and reversible.

## Security by Default

Local inference should not accidentally become a publicly accessible network service.

Networking and remote execution should be deliberate features with appropriate security.

## Measure Reality

Do not assume something works because an API theoretically supports it.

Test:

- model loading
- memory consumption
- tool calling
- context length
- agent behavior
- inference speed
- portability
- hardware detection
- compatibility

---

# WHAT I WANT FROM YOU

Treat this document as the project's North Star rather than as an implementation specification.

You are responsible for determining the best technical approach.

Do not blindly follow an architecture I have not specified.

Do not assume llama.cpp, Ollama, MLX, Electron, Go, Rust, Python, or any other technology must be used unless evidence shows it is the right choice.

Research the current ecosystem.

Evaluate alternatives.

Make architectural decisions based on:

- simplicity
- portability
- performance
- maintainability
- compatibility
- licensing
- long-term extensibility

When you make an important architectural decision, document:

1. the problem
2. the alternatives considered
3. the decision
4. why you selected it
5. what tradeoffs it creates

Do not build all phases now.

The following Phase 1 instructions are retained as the original starting brief. For current work, follow the agreed milestones at the top of this document.

Before implementing Phase 1:

1. inspect the existing repository, if any
2. research the best current technical options
3. identify the minimum system required to prove Phase 1
4. propose your approach
5. identify the major risks or unknowns
6. define clear acceptance tests
7. then implement it

Do not turn Phase 1 into a prototype full of fake interfaces for functionality that does not exist.

Build the simplest real implementation that proves the concept while preserving a reasonable path toward the North Star.

The first question you are trying to answer is:

> Can we carry the model and inference environment on an external drive, connect it to a compatible computer, use that computer's available AI hardware, and perform useful local inference without requiring a conventional local inference installation?

Prove that first.

Everything else follows from that.
