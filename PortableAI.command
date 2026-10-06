#!/bin/zsh -f
set -eu
ROOT=${0:A:h}
if [[ $(/usr/bin/uname -m) != arm64 ]]; then
  print -u2 'Reflect Go requires an Apple Silicon Mac running natively (not Rosetta).'
  exit 1
fi
if [[ ! -x "$ROOT/bin/portableai" ]]; then
  print -u2 'Missing launcher. Open Reflect Go.command inside an assembled phase folder.'
  exit 1
fi
exec "$ROOT/bin/portableai" "$@"
