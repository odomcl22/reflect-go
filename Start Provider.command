#!/bin/zsh -f
set -eu
ROOT=${0:A:h}
exec "$ROOT/PortableAI.command" serve "$@"
