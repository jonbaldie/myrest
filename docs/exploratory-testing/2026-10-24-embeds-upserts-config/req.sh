#!/bin/sh
# Log one HTTP call and echo it. Usage: req.sh LABEL curl-args...
set -u
LABEL=$1
shift
BASE=${BASE:-http://127.0.0.1:3124}
LOG=${LOG:-docs/exploratory-testing/2026-10-24-embeds-upserts-config/log.txt}
{
  echo
  echo "### $LABEL"
  echo "\$ curl -sS -D - $*"
} | tee -a "$LOG"
curl -sS -D - "$@" | grep -v '^Date:' | tee -a "$LOG"
echo | tee -a "$LOG"
