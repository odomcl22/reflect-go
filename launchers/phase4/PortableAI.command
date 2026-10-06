#!/bin/sh
ROOT=$(CDPATH= cd -- "$(dirname -- "$0")" && pwd)
if [ "$#" -eq 0 ]; then set -- chat; fi
exec "$ROOT/bin/portableai" "$@"
