# Reflect Go harness compatibility

Research checked October 10, 2026. Availability of an installed application, support for a configurable endpoint, and a successfully tested Reflect Go integration are separate facts. The product goal is a remembered model selection and one-click launch. An adapter is supported only after its launch, model discovery, inference, cancellation, and recovery are tested.

## What the Ollama screenshots establish

Ollama's Apps screen connects installed desktop applications as well as terminal clients. Its Claude settings map fixed Claude model options to selected inference models; its ChatGPT settings add up to five models to **Codex mode**. These are real desktop integrations, not evidence that all desktop applications accept the same launch flags. The examples in the screenshots are cloud models; Reflect Go's intended path uses local weights.

Ollama documents connection, restart, model selection, and restoration for [Claude Desktop](https://docs.ollama.com/integrations/claude-desktop) and [ChatGPT Desktop](https://docs.ollama.com/integrations/chatgpt). Their desktop applications need to be installed on the host. Installing or redistributing them on a portable drive is a separate packaging question.

## Integration matrix

| Harness | Relevant connection mechanism | Reflect Go status |
|---|---|---|
| Codex CLI | Responses API, custom provider, generated model catalog, isolated `CODEX_HOME` | Existing portable connector; actual local responses tested. |
| Claude Code CLI | Anthropic Messages endpoint and model environment variables | Existing portable connector; actual local responses tested. |
| OpenCode CLI | OpenAI Chat Completions provider and model list via `OPENCODE_CONFIG_CONTENT` | Existing portable connector; actual local responses tested. A 16K context minimum avoids the observed 8K compaction loop. |
| DeepSeek Harness | `dsh web --patch` supplies an owned settings provider, including a model catalog and local OpenAI-compatible endpoint | New host-installed adapter. Installed 0.1.1-rc.2 served its graphical UI and selected model catalog in an isolated profile. Full coding workflows remain unqualified. Portable `dsh` and Node packs are not included. |
| Claude Desktop | Third-party inference profile, Anthropic catalog/alias gateway, app restart, configuration restoration | Experimental macOS adapter with configuration/protocol tests. Real desktop conversations and coding workflows remain unqualified; CLI success alone is insufficient. |
| ChatGPT Desktop / Codex mode | Combined native/local catalog plus a dedicated Responses router | Exact upstream mechanism identified; Reflect Go desktop adapter not implemented yet. Ordinary ChatGPT chat and voice are not redirected by this integration. |
| OpenCode Desktop | Built-in server inherits provider configuration, or desktop connects to an external OpenCode server | External-server connection is documented; a fully automated, reversible desktop connection remains unqualified. OpenCode's graphical browser client can use the same isolated provider configuration as its CLI. |
| OpenCode Web | Portable OpenCode server with the existing local provider configuration and a protected browser connection | New graphical adapter. The 1.18.34 pack served its actual UI and selected model catalog in an isolated profile. Browser/tool workflows remain to be qualified. |
| Antigravity SDK | `LocalOpenAIAgentConfig(model, base_url)` connects to a local OpenAI-compatible server | Official local-model route identified. This does not establish support in the Antigravity desktop/IDE model picker. |
| Antigravity desktop, IDE, CLI | Official model picker currently documents supplied models; no supported arbitrary endpoint setting found in the reviewed docs | Research target. Do not patch the installed application or claim local compatibility from SDK support alone. |

## Desktop implementation findings

### Claude Desktop

Ollama's [profile adapter](https://github.com/ollama/ollama/blob/main/cmd/launch/claude_desktop.go) sets `deploymentMode` to `3p` and supplies a configuration-library profile with `inferenceProvider: gateway`, a loopback URL, bearer credentials, and a display name. It restores the regular profile on disconnect. Its [gateway](https://github.com/ollama/ollama/blob/main/internal/proxy/claude_desktop.go) serves model discovery and Anthropic Messages/count-tokens requests. Its [model adapter](https://github.com/ollama/ollama/blob/main/internal/proxy/claude_desktop_models.go) maps fixed Claude-facing route IDs to selected models. Reflect Go must preserve existing profiles and detect concurrent edits before restoring them. It must not inherit Ollama's cloud web search or enable automatic tool approvals merely to reproduce a model connection.

### ChatGPT Desktop / Codex mode

The current [Ollama adapter](https://github.com/ollama/ollama/blob/main/cmd/launch/codex_app.go) merges local entries with the native catalog and sets `model_catalog_json` plus `openai_base_url`, leaving `model_provider` unset to preserve native account controls. Its [desktop router](https://github.com/ollama/ollama/blob/main/internal/proxy/codex_desktop.go) handles selected local models and native-provider requests differently. Pointing the whole desktop app at Reflect Go's existing inference-only server would not reproduce that behavior. OpenAI's [app-server documentation](https://developers.openai.com/siwc/token-sharing-open-source/codex-app-server) establishes the Responses transport and catalog-versus-access distinction, but is not a guarantee that a custom provider appears in every desktop release.

### OpenCode Desktop

OpenCode documents [connecting Desktop to an external OpenCode server](https://docs.opencode.ai/docs/windows-wsl/). That server can have Reflect Go as its inference provider. The installed desktop version inspected was 1.18.10. Its [matching startup source](https://github.com/anomalyco/opencode/blob/v1.18.10/packages/desktop/src/main/index.ts) assigns its own Electron user-data path; a generic `--user-data-dir` launch is therefore not sufficient to establish isolation. Its [server startup](https://github.com/anomalyco/opencode/blob/v1.18.10/packages/desktop/src/main/server.ts) imports login-shell environment values, so a desktop adapter must verify that host settings cannot replace the intended endpoint. The supported [web application](https://docs.opencode.ai/docs/web/) is a separate graphical option, launched from the portable OpenCode executable.

The new OpenCode Web adapter runs `serve` with a random loopback port and a private server credential. A Reflect Go-owned browser bridge establishes an HttpOnly, same-site session before forwarding requests; neither the server password nor the inference key enters the browser URL. Web settings and sessions stay on the drive. OpenCode Web and DeepSeek processes launched by Reflect Go stop when Reflect Go closes, so they do not silently retain the drive after exit. This lifecycle does not terminate host-installed desktop applications.

### DeepSeek Harness

DeepSeek's [provider guide](https://github.com/deepseek-ai/deepseek-harness/blob/master/docs/user/guide/providers.md) supports custom Chat Completions, Responses, and Anthropic providers and selected model catalogs. Reflect Go uses Chat Completions, text-only model declarations, actual context/output limits, and no invented reasoning levels. Its configuration files contain a credential environment-variable name, not the credential. `DSH_HOME`, project sessions, and launch settings stay on the portable drive; the user's existing DeepSeek settings are not edited. The launch patch disables cloud web search and telemetry and retains workspace-write tool approvals. [Ollama's implementation](https://github.com/ollama/ollama/blob/main/cmd/launch/deepseek_harness.go) confirms the settings-provider patch mechanism. No automatic npm installation is performed by Reflect Go.

### Antigravity

Google explicitly documents [local OpenAI-compatible servers in its SDK](https://antigravity.google/docs/sdk/local-models/). The SDK's lightweight configuration is a possible future adapter, with authentication and tool behavior still requiring validation against Reflect Go. Its [desktop model documentation](https://antigravity.google/docs/models/) describes the supplied model picker. The reviewed documentation does not establish an arbitrary local endpoint setting for the desktop, IDE, or `agy` CLI. SDK compatibility must not be marketed as desktop integration.

## Qualification and connection lifecycle

Each supported adapter needs evidence for selected-model visibility, default selection, real tool requests, cancellation, engine/model changes, and launch failures. Desktop adapters also need restart consent when an existing session would be interrupted, a connection record, restoration of only the settings Reflect Go owns, and refusal to overwrite settings changed by another application. Authentication and tool approval policies must remain intact. A host-installed app may keep its own data on the computer even when weights remain on the portable drive; each integration must state where it stores state.
