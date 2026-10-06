#!/bin/sh
ROOT=$(CDPATH= cd -- "$(dirname -- "$0")" && pwd)
exec /bin/sh "$ROOT/Reflect Go.sh" qualify "$@"
