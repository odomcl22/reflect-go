# Reflect Go — Phase 4 (0.4.0)

Your models travel on the drive. This computer supplies the CPU/GPU and memory.
Phase 4 adds native Windows, macOS and Linux launchers and adaptive runtime packs.
The software runs from this folder without installing an inference engine, Python,
Node or Go on the host. Keep `Phase 4` beside the shared `Models` folder.

## Start here

- **Mac:** open `Reflect Go.app` (or `Reflect Go.command`). Select a model, click
  **Start engine**, then **Quick test**. `Browser Workstation.command` opens the
  optional browser interface.
- **Windows:** double-click `Reflect Go.cmd`. Keep its console open. The workstation
  opens in your existing browser. Select a model, start it and run Quick test.
- **Linux:** run `sh "Reflect Go.sh"` in a terminal. Keep that terminal open.
  The workstation opens in your existing browser. If automatic opening fails,
  open the displayed `state/OPEN-WORKSTATION.html` file while the launcher runs.

The browser interface and inference API bind only to `127.0.0.1`; no internet
connection is needed for inference. Browser controls use a separate session token.
Closing a browser tab leaves the engine running. Use **Quit workstation** before
closing its terminal or ejecting. The Mac app stops its engine when you quit.
Each phase owns only the processes it starts; other engines are left alone.

## Platform coverage and requirements

| Computer | Packaged runtime | Status |
| --- | --- | --- |
| Apple Silicon Mac | Metal and CPU | Tested on M1; see VALIDATION.md |
| Intel Mac | Metal where available, CPU fallback | Built; physical qualification pending |
| Windows x86-64 | Vulkan and CPU | Built; physical qualification pending |
| Windows ARM64 | CPU | Built; physical qualification pending |
| Linux x86-64 / ARM64 | Vulkan and CPU | Built; physical qualification pending |
| Chromebook / ChromeOS | Linux runtime inside enabled Linux environment | Conditional; device test pending |

Mac requires macOS 13.3 or later. Windows targets 64-bit Windows 10/11. Linux
requires glibc (not Alpine/musl); exact library floors are recorded per runtime in
`runtime-targets.json` and `dependency-audit.json`. Ubuntu 24.04 and Debian 13 are
the intended qualification baselines. GCC/OpenMP and OpenSSL runtime libraries are bundled.
Vulkan requires a working host GPU driver and Vulkan loader. The optional Windows
Vulkan pack also needs existing MSVC 2015–2022 runtime libraries. If those are
absent, Auto uses the self-contained Windows CPU runtime, built with LLVM-MinGW.
Its generic CPU baseline trades peak throughput for broad instruction support. A browser is needed
for the browser interface; direct CLI inference does not need one.

A Chromebook must have Linux enabled and permission to share/access the drive.
The Linux environment must be a supported 64-bit architecture with sufficient
memory and executable storage. Managed-device policy can prevent this. If the
shared mount is `noexec`, copy the whole collection into an executable Linux
location or use an administrator-approved mount. This is a conditional portable
Linux workflow, not a native ChromeOS application or a universal Chromebook claim.

Auto probes the packaged runtime for usable Metal/Vulkan devices and falls back
to CPU if acceleration cannot start or the conservative GPU memory screen
fails. Explicit backend selections fail visibly rather than silently changing.
The model screen uses half of physical/container-limited RAM and estimated model,
cache and working memory. It is not a free-memory measurement or a fit guarantee.
No automatic model substitution occurs after you explicitly select a model.
Lower context or use the smaller model if memory is tight. A failed real GPU load
can still require selecting CPU manually; probe success does not prove model fit.

## Terminal and client workflows

`Start Provider` starts the API in a terminal. Wait for the listening message.
Use the workstation's client buttons for existing **Codex CLI** or **Claude Code
CLI**, with a project folder outside this collection. Claude can ask whether to
use the supplied local API key: choose Yes for this local connection. Clients keep
their normal tool/project permissions. The model runs locally; client software can
have its own network behavior. Neither harness nor model capability is universal.

Portable harness packs, OpenCode integration and broad client version qualification
remain **Phase 4.5**. This release does not bundle those CLIs or any desktop harness.
Windows adapters currently require a native `.exe` client; npm `.cmd` wrappers are
not supported by this preview.

For direct commands, invoke `Reflect Go.sh` on Linux/Mac or `Reflect Go.cmd` on
Windows, followed by one of:

```
models
chat
run --prompt "Say hello" --max-tokens 64
serve --context 16384 --port 8766
serve --backend cpu --context 4096
connect codex --project /path/to/project --task "Describe this project"
connect claude --project /path/to/project
doctor
verify
qualify --model qwen2.5-1.5b-instruct-q4_k_m.gguf --context 2048
```

`Verify Portability` performs a real load, authenticated API/metrics checks,
streaming generation, a quick response and shutdown. It saves a credential-free
`state/qualification.json` report for **this host only**. Run with all other engines
stopped and adequate RAM; it reads all packaged files and both model hashes first.
Use `--backend cpu` to test fallback separately. Reports do not qualify other PCs.

## Storage, integrity and troubleshooting

Models are stored once in `../Models`; none are copied into this phase. Mutable
settings, logs, provider keys and inference caches stay under `state`. The host OS,
browser and installed clients may keep their own normal caches/history. Never share
`state` or provider keys. Backups live only in the source project's `releases`
folder and exclude models and state; there are no backup archives on this drive.

`verify` checks the software/model manifest. Added models receive format/memory
screening but have no known hash until you add a trusted manifest entry. Keep the
whole phase intact. A busy API port requires another port or stopping your other
engine. Diagnostics includes the exact runtime probe failure and requirements.
Executable drive permissions, OS trust prompts, missing GPU drivers, RAM pressure
and unsupported CPU instructions can prevent launch. This preview is not signed
with a developer distribution certificate; follow normal OS verification, without
disabling system security protections.

Pinned upstream: llama.cpp b11146. Runtime, compiler and Ubuntu package hashes are
in `runtime-targets.lock.json`; notices are under `licenses`. Go is only a build-time
dependency. No models or executables are downloaded at application startup.
