# Codex Desktop test handoff

Finish the native Codex desktop connection test from a separate controller, such as Claude Code. Test the existing implementation first; fix reproducible failures without replacing the working desktop adapters.

## Starting point

- Repository: `odomcl22/reflect-go`. Desktop fixes are in commit `196b7be` and later.
- The locally patched Mac package is `dist/Reflect Go/Phase 4.8/`. Open its `Reflect Go.app`, keeping its support folders together.
- The published `v0.4.8-preview` ZIPs do **not** contain these follow-up fixes. Do not use those ZIPs to test the fixed behavior.
- Test model: `Qwen3-4B-Instruct-2507-Q4_K_M.gguf`, llama.cpp/Metal, 49,152 context tokens on a 16 GiB M1 Mac. The Starter Pack's Qwen2.5 1.5B model cannot satisfy this desktop context requirement.
- At handoff, Reflect Go was quit, its owned services stopped, and Claude/OpenCode settings restored. The removable drive was not mounted and was not updated.

## What was actually tested

Messages were entered and sent through the real application interfaces. These were not merely HTTP health checks or screenshots of configuration screens.

| Application | Message sent | Visible reply | Result |
| --- | --- | --- | --- |
| Claude Desktop 2.31226.1 | `Connection check: reply exactly REFLECT GO LOCAL OK. Do not use tools or read any files.` | `REFLECT GO LOCAL OK` | Native window reply passed; about 492 seconds because Claude sent a 33,820-token initial prompt. |
| OpenCode Desktop 1.18.35 | `Updated version check: reply exactly OPENCODE UPDATED OK. Do not use tools, run commands, or read or change files.` | `OPENCODE UPDATED OK` | Native window reply passed in 57 seconds without refresh/restart. |
| DeepSeek Harness 0.1.1-rc.2 | `Connection check: reply exactly DEEPSEEK READY. Do not use tools, run commands, or read or change files.` | `DEEPSEEK READY` | Browser reply passed in 51 seconds after a fresh native Reflect Go launch, with the project and model selected automatically. |

The responses were correlated with the local llama.cpp engine log. OpenCode's earlier 1.18.10 test completed inference but needed a restart to display the reply; the fresh test above used the official 1.18.35 update. Claude Disconnect/reconnect/Quit and OpenCode Disconnect/Quit restored their journaled fields without signing out.

Private screenshots are retained locally under `.build/desktop-qa-20261010/`: `opencode-updated-ok.png` and `deepseek-ready.jpg`. No saved Claude screenshot is part of this handoff; its reply was observed in the native accessibility tree. Do not publish raw profiles, logs, journals, or screenshots containing unrelated projects/account information.

Codex's **installed app-server** discovered the offered model and completed a real local turn in about 14 seconds using a temporary profile. That test did not exercise the running desktop window, its model picker, or the real account's restart/restoration cycle. Those remain the task here. See [the full validation report](PREVIEW_0_4_8_VALIDATION.md#follow-up-desktop-testing).

## Use the correct desktop application

On this Mac the installed targets were:

| Application | Bundle ID | Version | Test role |
| --- | --- | --- | --- |
| `/Applications/ChatGPT.app` | `com.openai.codex` | `26.1002.52244` | The Codex desktop adapter's actual target. |
| `/Applications/ChatGPT Classic.app` | `com.openai.chat` | `1.2026.184` | A separate application; testing it does not qualify this connector. |

The adapter does not redirect ordinary ChatGPT chat or voice. Test **Codex mode with a new task and an explicitly selected Reflect Go model**. The names/versions above describe the inspected installation; confirm them again if the apps have been updated or moved.

Run the test controller outside the target ChatGPT/Codex app. Finish active Codex tasks before its restart. Keep the controller on its normal connection so a Reflect Go failure or disconnect cannot interrupt the controller too.

## Native test: exact steps

1. Open the patched `dist/Reflect Go/Phase 4.8/Reflect Go.app`. The footer should show `0.4.8-preview`; the installed backend must include commit `196b7be`'s fixes, since the version label alone does not distinguish the old ZIP.
2. In **Models**, offer the Qwen3 4B model above and make it the default. For this first test, offer only that one model. The service can start automatically when connecting.
3. In **Apps → More apps**, choose **Connect Codex Desktop**. Confirm the desktop restart after current work is finished. If a smaller-context service is already running, finish its client tasks and allow Reflect Go to restart it at 49,152 tokens.
4. In the reopened ChatGPT/Codex app, start a **new Codex task**. Use a disposable empty project folder selected inside the desktop app. Desktop apps currently choose their own projects; Reflect Go's project field applies to CLI/web adapters.
5. Inspect the model picker. Select **Reflect Go · Qwen3-4B-Instruct-2507-Q4_K_M.gguf**. Record the displayed model and the selected `reflect-…` ID if available. Do not send the test using a cloud model or assume an existing task changed its saved selection.
6. Send this exact message:

   > Connection check: reply exactly CODEX LOCAL OK. Do not use tools, run commands, or read or change files.

7. Verify the **native conversation displays an assistant reply** and finishes without refresh or restart. Expected text: `CODEX LOCAL OK`. Correlate the request time, selected model and new prompt/evaluation activity in the phase's `state/workstation-engine.log`. A model listing, a configured URL, or the model claiming to be local is insufficient evidence.
8. Send `Reply exactly CODEX SECOND OK. Do not use tools.` Verify a second visible completion. Record cold and warm response times separately. If the first attempt is slow, check whether prompt-processing progress is advancing before declaring a timeout.
9. Test cancellation with a harmless long response request, then use the desktop's Stop control. Check that it stops and a subsequent short prompt succeeds. Record any retries, stuck UI, or engine errors.
10. Finish the task, then use Reflect Go's **Disconnect** for Codex. Allow the normal restart. Verify the previous model/provider settings and account access return without signing out. Inspect restoration locally; never paste credentials into the report.
11. Connect again, send another short local message in a fresh task, then **Quit Reflect Go**. Verify the same restoration, the removal of the current phase's `state/codex-desktop-restore.json`, and shutdown of Reflect Go's owned services. A restoration conflict must be reported and preserved, not resolved by deleting the journal or overwriting another application's changes.

If tools are to be qualified as well, use only the disposable project: ask Codex to create `reflect-go-smoke.txt` containing `LOCAL TOOL OK`, read it back, and stop. Retain normal approval/sandbox controls. Verify the file on disk and ensure no other project was touched. Record this separately from the message-only test; a reply does not prove a coding workflow works.

## How the connections are implemented

| Component | Implementation and relevant source |
| --- | --- |
| Shared launch | [`integrations.go`](../Sources/ReflectGo/integrations.go): `launch` selects offered models, enforces the desktop context requirement and waits for the local service. `launchDesktop` creates an adapter/proxy and a profile-change journal, then `runDesktopChange` handles normal quit, apply, restart and rollback. |
| Saved connection/readiness | [`clients.go`](../Sources/ReflectGo/clients.go): `connection` validates the loopback provider and loads its key privately. The previous 32,768-token validation ceiling conflicted with desktop launch's 49,152-token context; it now accepts the engine's range up to 131,072. [`provider_connection_test.go`](../Sources/ReflectGo/provider_connection_test.go) covers the actual saved-record/readiness path. |
| Claude Desktop | [`desktop_claude.go`](../Sources/ReflectGo/desktop_claude.go): reversible third-party profile with display name Reflect Go. An authenticated Anthropic proxy exposes `/v1/models`, `/v1/messages` and token counting; native Claude model slots map to offered local IDs. Local replies are bounded at 4,096 tokens and unsupported effort controls are removed. Inactive profile defaults written by Claude are preserved on reconnect. |
| OpenCode Desktop | [`desktop_apps.go`](../Sources/ReflectGo/desktop_apps.go): `planDesktopProfile` adds `provider/reflect_go`, `model` and `small_model` to the existing JSON/JSONC config. It uses an OpenAI-compatible endpoint at the local provider's `/v1`. Existing providers, account settings, comments and permissions are preserved. Existing conversations/project settings may override the global default, so inspect the picker. |
| DeepSeek Harness | [`integration_deepseek.go`](../Sources/ReflectGo/integration_deepseek.go): launches installed `dsh` with an isolated `DSH_HOME`, selected model catalog, credential environment variable and local endpoint. It finds Node in normal installation locations when Finder's PATH omits it, registers the chosen workspace through the local harness API, then opens the browser. Its process is owned and stopped by Reflect Go. It does not reuse the user's Ollama-launched instance on port 3080. |
| Codex Desktop profile | [`desktop_apps.go`](../Sources/ReflectGo/desktop_apps.go): merges existing native catalog entries with `codexDesktopModel` entries. Journals changes to `~/.codex/config.toml`: `model_catalog_json`, `openai_base_url`, and `model`; temporarily clears `model_provider`/`profile` overrides. Writes `~/.codex/reflect-go-models.json`. Existing `auth.json` is retained. If no auth file exists, a temporary local-only placeholder is created and journaled for removal. |
| Codex Desktop routing | [`desktop_codex.go`](../Sources/ReflectGo/desktop_codex.go): `startCodexDesktopProxy` creates a random loopback listener with a private URL prefix. Offered local model IDs route to llama.cpp's Responses endpoint. Local requests discard account headers, substitute the local key, remove account-only effort/service controls and bound output. Existing cloud model selections use their normal provider/account route; local requests never fall back to cloud. The private URL contains a credential: redact it from evidence. |
| Restore/disconnect | [`managed_profile.go`](../Sources/ReflectGo/managed_profile.go) journals only owned fields and checks for later external edits before restoring. [`integrations.go`](../Sources/ReflectGo/integrations.go) restores on Disconnect and normal Quit, restarting apps that were open. Never replace the whole user config or remove sign-in credentials to make a test pass. |

Known Codex limitations: this router uses HTTP Responses, rejects WebSocket upgrades, and does not implement local `/responses/compact`. Start fresh tasks for testing. Local catalog entries do not advertise unsupported reasoning levels, cloud search, or parallel tool calls. Do not describe those controls as working merely because a desktop UI displays them.

## Repeat the isolated app-server check if needed

This is a diagnostic supplement, not a replacement for the native steps above. Start Reflect Go's local service with at least 49,152 context tokens first. From the repository root, use the following to run the existing opt-in test with the installed desktop binary and saved provider record. The script passes only the key-file path, not its contents, and does not edit the live desktop profile.

```sh
python3 - <<'PY'
import json
import os
from pathlib import Path
import subprocess

repo = Path.cwd()
phase = repo / "dist/Reflect Go/Phase 4.8"
provider = json.loads((phase / "state/provider.json").read_text())
if provider.get("context", 0) < 49152:
    raise SystemExit("Restart the local service with at least 49,152 context tokens before this desktop test.")
binary = Path("/Applications/ChatGPT.app/Contents/Resources/codex-cli/CodexCLI.app/Contents/MacOS/codex")
go = repo / ".build/toolchains/go/bin/go"
if not go.is_file() or not binary.is_file():
    raise SystemExit("Locate the installed Go toolchain and Codex desktop binary before running this test.")
env = os.environ.copy()
env.update({
    "GOMODCACHE": str(repo / ".build/go-mod-cache"),
    "GOCACHE": str(repo / ".build/go-cache"),
    "GOTOOLCHAIN": "local",
    "REFLECT_GO_TEST_CODEX_BINARY": str(binary),
    "REFLECT_GO_TEST_ENGINE_BASE": provider["base_url"],
    "REFLECT_GO_TEST_ENGINE_KEY_FILE": str(phase / "state/provider-key"),
    "REFLECT_GO_TEST_MODEL_ID": provider["model"],
})
subprocess.run([
    str(go), "-C", "Sources/ReflectGo", "test", "-count=1", "-v",
    "-run", "^TestInstalledCodexDesktopCatalog$", ".",
], env=env, check=True)
PY
```

[`TestInstalledCodexDesktopCatalog`](../Sources/ReflectGo/desktop_connections_test.go) starts the installed `codex app-server` with a temporary profile, initializes it, checks `model/list`, creates an ephemeral read-only thread, sends a prompt, and requires assistant text events plus a completed turn. It currently does not assert the exact text `OK`, exercise the desktop window, or qualify file-editing tools. Merely running the Go suite without the opt-in environment skips this live test.

For code changes, run the affected tests and the full Go suite. Relevant regression coverage includes `TestCodexDesktopRoutesLocalWithoutAccountCredential`, `TestDesktopProfilesRestoreOnlyOwnedFields`, `TestDesktopModelPickerDoesNotBlockDisconnect`, the Claude profile/proxy tests, the DeepSeek launch tests, and the restart/cancellation/rollback tests in `integrations_launch_test.go`.

## Return a concrete result

Update the validation report with the exact app/build/model, native picker selection, prompt and visible reply, cold/warm timing, local-engine correlation, cancellation result, and Disconnect/reconnect/Quit restoration. Record message-only and tool-use results separately. Save sanitized screenshots and retain raw diagnostic evidence locally outside Git.

If anything fails, report the exact native error and matching local request/engine evidence, fix the smallest demonstrated cause, and repeat the failed step. Preserve account settings, project permissions, and the working adapters. Do not publish refreshed binaries or mark the whole desktop workflow qualified solely on the basis of a passing app-server test.
