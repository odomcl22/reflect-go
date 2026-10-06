#!/bin/zsh -f
set -eu
ROOT=${0:A:h}
source "$ROOT/bin/client-common.zsh"
if [[ $# -gt 0 ]]; then shift; fi
CLIENT=$(command -v codex || true)
if [[ -z "$CLIENT" && -x "$HOME/.local/bin/codex" ]]; then CLIENT="$HOME/.local/bin/codex"; fi
if [[ -z "$CLIENT" ]]; then
  print -u2 'This connector needs an existing Codex CLI installation. Reflect Go provides inference, not the client.'
  exit 1
fi
if [[ $# -gt 0 ]]; then
  TASK=$1
else
  print 'Task for the local coding agent:'
  read -r TASK
fi
if [[ -z "$TASK" ]]; then print -u2 'No task entered.'; exit 1; fi
export REFLECT_GO_API_KEY="$PROVIDER_KEY"
print -r -- "Codex CLI → Reflect Go ($PROVIDER_URL) · $PROJECT"
print 'This run can edit the selected project and run commands within the Codex workspace sandbox.'
exec "$CLIENT" exec --ignore-user-config --ephemeral --skip-git-repo-check \
  --sandbox workspace-write --color never -C "$PROJECT" \
  -c 'model="reflect-go-local"' -c 'model_provider="reflect_go"' \
  -c 'model_providers.reflect_go.name="Reflect Go"' \
  -c "model_providers.reflect_go.base_url=\"$PROVIDER_URL/v1\"" \
  -c 'model_providers.reflect_go.env_key="REFLECT_GO_API_KEY"' \
  -c 'model_providers.reflect_go.wire_api="responses"' \
  -c 'model_providers.reflect_go.requires_openai_auth=false' \
  -c 'model_providers.reflect_go.supports_websockets=false' \
  -c 'model_providers.reflect_go.request_max_retries=0' \
  -c 'model_providers.reflect_go.stream_max_retries=0' \
  -c 'model_providers.reflect_go.stream_idle_timeout_ms=180000' \
  -c "model_context_window=$PROVIDER_CONTEXT" \
  -c "model_auto_compact_token_limit=$((PROVIDER_CONTEXT * 85 / 100))" \
  -c "model_instructions_file=$(toml_quote "$ROOT/config/codex-local-instructions.txt")" \
  -c 'model_supports_reasoning_summaries=false' -c 'web_search="disabled"' \
  -c 'approval_policy="never"' -c 'features.unified_exec=false' \
  -c 'features.plugins=false' -c 'features.apps=false' -c 'features.multi_agent=false' \
  -c 'shell_environment_policy.inherit="core"' -c 'shell_environment_policy.ignore_default_excludes=false' \
  -c 'allow_login_shell=false' -- "$TASK"
