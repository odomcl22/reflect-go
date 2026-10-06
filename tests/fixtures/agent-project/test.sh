#!/bin/sh
set -eu
cd "$(dirname "$0")"
. ./math.sh
check() {
    actual=$(clamp "$1" "$2" "$3")
    test "$actual" = "$4" || { printf 'FAIL: clamp %s %s %s: expected %s, got %s\n' "$1" "$2" "$3" "$4" "$actual"; exit 1; }
}
check -5 0 10 0
check 5 0 10 5
check 15 0 10 10
check 0 0 10 0
check 10 0 10 10
printf 'All five clamp tests passed.\n'
