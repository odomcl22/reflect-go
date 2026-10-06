#!/bin/zsh -f
set -eu
ROOT=${0:A:h}
source "$ROOT/bin/client-common.zsh"
if [[ $# -gt 0 ]]; then shift; fi
CLIENT=$(command -v claude || true)
if [[ -z "$CLIENT" && -x "$HOME/.local/bin/claude" ]]; then CLIENT="$HOME/.local/bin/claude"; fi
if [[ -z "$CLIENT" ]]; then
  print -u2 'This connector needs an existing Claude Code installation. Reflect Go provides inference, not the client.'
  exit 1
fi
export ANTHROPIC_API_KEY="$PROVIDER_KEY"
export ANTHROPIC_BASE_URL="$PROVIDER_URL"
export ANTHROPIC_MODEL=reflect-go-local ANTHROPIC_DEFAULT_HAIKU_MODEL=reflect-go-local
export ANTHROPIC_DEFAULT_SONNET_MODEL=reflect-go-local ANTHROPIC_DEFAULT_OPUS_MODEL=reflect-go-local
export ANTHROPIC_SMALL_FAST_MODEL=reflect-go-local
export CLAUDE_CODE_DISABLE_NONESSENTIAL_TRAFFIC=1 DISABLE_PROMPT_CACHING=1
export CLAUDE_CODE_MAX_OUTPUT_TOKENS=2048 MAX_THINKING_TOKENS=0
print -r -- "Claude Code → Reflect Go ($PROVIDER_URL) · $PROJECT"
print 'If Claude asks to use the supplied API key (even with No recommended), choose Yes for this Reflect Go local connection.'
print 'Claude Code retains its normal project/tool permission prompts. Model quality and context are limited by the local model.'
cd "$PROJECT"
exec "$CLIENT" --bare --setting-sources '' --strict-mcp-config --mcp-config '{"mcpServers":{}}' \
  --disable-slash-commands --no-chrome --model reflect-go-local \
  --append-system-prompt "$(/bin/cat "$ROOT/config/claude-local-instructions.txt")" \
  --tools Read,Edit,Write,Bash,Glob,Grep --permission-mode manual "$@"
