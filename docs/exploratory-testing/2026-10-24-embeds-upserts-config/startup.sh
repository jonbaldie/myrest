#!/bin/sh
# Start myrest with one config file and env, wait up to 3s, log the outcome.
# Usage: startup.sh LABEL CONFIG-FILE [env assignments...]
set -u
LABEL=$1; CONF=$2; shift 2
LOG=${LOG:-docs/exploratory-testing/2026-10-24-embeds-upserts-config/startup-log.txt}
PORT=${PORT:-3126}
{
  echo; echo "### $LABEL"; echo "\$ env $* MYREST_LISTEN=127.0.0.1:$PORT ./bin/myrest $CONF"
  [ -f "$CONF" ] && sed 's/^/  | /' "$CONF"
} | tee -a "$LOG"
OUT=$(mktemp)
env "$@" MYREST_LISTEN=127.0.0.1:$PORT ./bin/myrest $CONF > "$OUT" 2>&1 &
PID=$!
i=0; while [ $i -lt 30 ] && kill -0 $PID 2>/dev/null; do
  if curl -s -o /dev/null http://127.0.0.1:$PORT/ 2>/dev/null; then break; fi
  sleep 0.1; i=$((i+1)); done
if kill -0 $PID 2>/dev/null; then
  echo "running; GET /items:" | tee -a "$LOG"
  curl -sS -i "http://127.0.0.1:$PORT/items?limit=1" 2>&1 | grep -v '^Date:' | tee -a "$LOG"; echo | tee -a "$LOG"
  kill $PID; wait $PID 2>/dev/null; echo "exit after kill" | tee -a "$LOG"
else
  wait $PID; echo "exit=$?" | tee -a "$LOG"
fi
sed 's/^/  > /' "$OUT" | tee -a "$LOG"; rm -f "$OUT"
