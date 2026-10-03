#!/bin/bash
set -e
DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/../../.." && pwd)"
cd "$DIR"
mkdir -p /tmp/et-myrest
cp "$DIR/docs/exploratory-testing/2026-10-03-aggregates-media-auth/myrest.conf" /tmp/et-myrest/myrest.conf
cp "$DIR/docs/exploratory-testing/2026-10-03-aggregates-media-auth/req.sh" /tmp/et-myrest/req.sh
pkill -f 'bin/myrest /tmp/et-myrest/myrest.conf' 2>/dev/null || true
pkill -f 'mysqlharness' 2>/dev/null || true
sleep 1
(MYREST_MYSQL_HARNESS_PORT=43306 go run ./cmd/mysqlharness ./testdata/fixtures/schema.sql > /tmp/et-myrest/harness.log 2>&1 &)
for i in $(seq 1 60); do
  if grep -q 'Press Ctrl' /tmp/et-myrest/harness.log 2>/dev/null; then
    break
  fi
  sleep 1
done
grep -q 'Press Ctrl' /tmp/et-myrest/harness.log || { echo "fixture load failed"; cat /tmp/et-myrest/harness.log; exit 1; }
(MYREST_LISTEN=127.0.0.1:3111 ./bin/myrest /tmp/et-myrest/myrest.conf > /tmp/et-myrest/server.log 2>&1 &)
sleep 2
head -5 /tmp/et-myrest/server.log
