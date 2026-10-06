#!/bin/sh
ROOT=$(CDPATH= cd -- "$(dirname -- "$0")" && pwd)
exec /usr/bin/open "$ROOT/Reflect Go.app"
