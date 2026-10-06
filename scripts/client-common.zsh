# Sourced by the two portable client connectors. Only macOS system tools are used.
if [[ -L "$ROOT/state" || -L "$ROOT/state/provider-key" || -L "$ROOT/state/provider.json" ]]; then
  print -u2 'Provider state must be ordinary files in this phase folder.'
  exit 1
fi
if [[ ! -f "$ROOT/state/provider.json" || ! -f "$ROOT/state/provider-key" ]]; then
  print -u2 'Start Provider.command first, and wait for its listening message.'
  exit 1
fi
PROVIDER_URL=$(/usr/bin/plutil -extract base_url raw -o - "$ROOT/state/provider.json")
PROVIDER_CONTEXT=$(/usr/bin/plutil -extract context raw -o - "$ROOT/state/provider.json")
PROVIDER_PORT=${PROVIDER_URL#http://127.0.0.1:}
if [[ "$PROVIDER_PORT" != <1024-65535> || "$PROVIDER_URL" != "http://127.0.0.1:$PROVIDER_PORT" || "$PROVIDER_CONTEXT" != <512-32768> ]]; then
  print -u2 'Invalid local provider settings. Restart Start Provider.command.'
  exit 1
fi
PROVIDER_KEY=$(<"$ROOT/state/provider-key")
if [[ ${#PROVIDER_KEY} != 64 || "$PROVIDER_KEY" == *[^0-9a-f]* ]]; then
  print -u2 'Invalid provider key. Restart the provider after restoring its state.'
  exit 1
fi
# Keep the bearer key out of the curl argument list and logs.
if ! { print -r -- "header = \"Authorization: Bearer $PROVIDER_KEY\""; } | \
  /usr/bin/curl --config - --noproxy '*' --fail --silent --show-error --max-time 5 \
  "$PROVIDER_URL/v1/models" > /dev/null; then
  print -u2 'The local provider is unavailable or still loading. Check its terminal.'
  exit 1
fi
if [[ -z "${1:-}" ]]; then
  print 'Project folder (paste its full path, without quotes):'
  read -r PROJECT
else
  PROJECT=$1
fi
PROJECT=${PROJECT:A}
if [[ ! -d "$PROJECT" ]]; then
  print -u2 'Project folder does not exist.'
  exit 1
fi
# Prevent accidental edits to the provider itself when launched in Finder.
if [[ "$PROJECT" == "$ROOT" || "$PROJECT" == "$ROOT/"* ]]; then
  print -u2 'Choose a project outside this phase folder.'
  exit 1
fi
unset OPENAI_API_KEY OPENAI_BASE_URL CODEX_API_KEY ANTHROPIC_API_KEY ANTHROPIC_AUTH_TOKEN
unset ANTHROPIC_BASE_URL CLAUDE_CODE_USE_BEDROCK CLAUDE_CODE_USE_VERTEX CLAUDE_CODE_USE_FOUNDRY
unset HTTP_PROXY HTTPS_PROXY ALL_PROXY http_proxy https_proxy all_proxy
export NO_PROXY=127.0.0.1,localhost
toml_quote() {
  local value="$1"
  value=${value//\\/\\\\}
  value=${value//\"/\\\"}
  value=${value//$'\n'/\\n}
  value=${value//$'\r'/\\r}
  value=${value//$'\t'/\\t}
  print -rn -- "\"$value\""
}
