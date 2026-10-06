#!/bin/sh
set -eu
ROOT=$(CDPATH= cd -- "$(dirname -- "$0")" && pwd)
case "$(uname -s)" in Linux) PLATFORM=linux;; Darwin) PLATFORM=macos;; *) echo 'Use Reflect Go.cmd on Windows.' >&2; exit 1;; esac
case "$(uname -m)" in aarch64|arm64) ARCH=arm64;; x86_64|amd64) ARCH=x64;; *) echo 'Reflect Go requires a 64-bit ARM or x86 computer.' >&2; exit 1;; esac
BINARY="$ROOT/bin/$PLATFORM-$ARCH/reflectgo"
if [ ! -x "$BINARY" ]; then
  echo 'This storage does not permit running Reflect Go. Use an executable mount or copy the collection to a writable, executable location; consult your device administrator when managed.' >&2
  exit 1
fi
exec "$BINARY" "$@"
