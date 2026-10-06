#!/bin/sh
ROOT=$(CDPATH= cd -- "$(dirname -- "$0")" && pwd)
PROJECT=${1-}
if [ -z "$PROJECT" ]; then printf 'Project folder: '; IFS= read -r PROJECT; fi
exec "$ROOT/bin/portableai" connect codex --project "$PROJECT"
