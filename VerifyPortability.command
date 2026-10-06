#!/bin/zsh -f
# Runs on the destination Mac using macOS tools only. Does not change host networking.
set -eu
ROOT=${0:A:h}
if [[ -L "$ROOT/state" ]]; then
  print -u2 'Refusing a symlinked state directory. Use the original portable folder.'
  exit 1
fi
mkdir -p "$ROOT/state"
REPORT=$(/usr/bin/mktemp -d "$ROOT/state/portability-check.XXXXXX")
print -r -- "Report: $REPORT"
print -r -- "Bundle: $ROOT" > "$REPORT/location.txt"
/bin/date -u >> "$REPORT/location.txt"
/bin/df -h "$ROOT" >> "$REPORT/location.txt"
"$ROOT/PortableAI.command" verify > "$REPORT/integrity.txt" 2>&1
"$ROOT/PortableAI.command" doctor > "$REPORT/hardware.json" 2> "$REPORT/hardware-errors.txt"
print 'Running real inference with network access denied for the test process. First launch can take a minute.'
if /usr/bin/sandbox-exec -p '(version 1)(allow default)(deny network*)' \
  /usr/bin/time -l "$ROOT/PortableAI.command" run --verbose --max-tokens 192 \
  --prompt 'Summarize this note in one sentence, preserving both days, both times, and online borrowing availability: The library will close at 3 pm on Friday for repairs and reopen Monday at 9 am. Online borrowing will remain available throughout.' \
  > "$REPORT/response.txt" 2> "$REPORT/runtime.log"; then
  /bin/cat "$REPORT/response.txt"
  print -r -- "Commands passed. Review response quality and GPU offload in $REPORT/runtime.log."
  print 'This run is recorded for this host only. Repeat on the external drive and second Mac before declaring portability proven.'
else
  print -u2 -r -- "Inference failed. Read $REPORT/runtime.log. The report is preserved."
  exit 1
fi
