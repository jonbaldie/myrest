#!/bin/sh
# Log one HTTP call. Usage: req.sh LABEL curl-args...
set -u
LABEL=$1
shift
BASE=${BASE:-http://127.0.0.1:3111}
LOG=${LOG:-docs/exploratory-testing/2026-10-10-profiles-reload-limits/log.txt}
{
  echo
  echo "### $LABEL"
  echo "\$ curl -sS -D - $*"
} >> "$LOG"
curl -sS -D - "$@" >> "$LOG"
echo >> "$LOG"
