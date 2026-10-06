# Phase 1 plan — 2026-10-03

The starting workspace was empty, without Git history or project instructions. The supplied North Star is preserved in NORTH_STAR.md. Only Phase 1 is authorized for implementation here.

## Minimum useful implementation

Produce `dist/PortableAI`, a relocatable directory containing a precompiled native launcher, a pinned macOS ARM64 llama.cpp runtime with its libraries, configuration, license notices, and one real GGUF text model. Finder opens a small `.command` entry point in Terminal. The native launcher detects hardware, offers eligible local models, and replaces itself with the runtime process. No server, package manager, Python, compiler, cloud credential, or download is required at destination.

The assembly computer may use Xcode command line tools and Python's standard library. These are build tools only. The distributed directory is the product. Hardware-specific flags live at a single process boundary so a later runtime can replace this backend without defining a speculative plugin framework.

## Risks and unknowns

- Binary deployment target, CPU instructions, library paths, and Metal resources must be checked, not inferred from an ARM64 filename.
- Gatekeeper/quarantine and volume execution permissions can prevent launch. Do not disable host security or silently clear quarantine.
- RAM estimates are screening heuristics. Runtime buffers and KV caches depend on model architecture; other applications can consume memory after inspection.
- Small-model quality must be assessed with useful tasks. This does not establish coding-agent competence.
- A path-relocation test is not proof of a removable-drive or second-host test. No external volume or second Mac is currently available.
- Filesystem formats, eject behavior, older supported macOS versions, and signing/notarization require physical release validation.

## Acceptance tests defined before implementation

1. Audit every packaged Mach-O dependency: only bundled paths and macOS frameworks/libraries; record architecture and minimum OS.
2. Report real host OS, CPU, physical RAM, and Metal device. Clearly report missing Metal; support an explicit CPU mode.
3. Discover local single-file GGUFs. Reject missing, malformed, external-symlink, split, and clearly oversized inputs before inference. Never download at launch.
4. Run the pinned model on Metal and CPU. Record actual replies, timing, memory, and runtime logs proving GPU offload.
5. Run useful summarization and code-generation prompts; evaluate the result rather than treating process success as model quality.
6. Rename/copy the complete directory to a path with spaces and non-ASCII characters. Launch with a clean environment, an empty HOME, and a system-only PATH. Deny network access during this test where supported.
7. Exercise interactive chat and exit, Ctrl-C during generation, and terminal hangup; verify the owned process is gone and no network listeners remain.
8. Test configuration errors, missing runtime/model, damaged downloads and model files, and conservative memory rejection.
9. Manually repeat on a writable removable drive and another compatible Mac with no development or inference tools installed, offline. Record model hashes, output, hardware, clean shutdown, and successful eject. Until performed, Phase 1's full portability success condition is not satisfied.
