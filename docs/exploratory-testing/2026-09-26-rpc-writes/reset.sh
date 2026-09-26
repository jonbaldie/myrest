#!/bin/bash
# Reload fixtures with the project harness into et-myrest-mysql, restart myrest on :3111.
cd /Users/jonathanbaldie/Code-2/github.com/jonbaldie/myrest
pkill -f 'bin/myrest /tmp/et-myrest/myrest.conf'; pkill -f 'mysqlharness'; sleep 1
(MYREST_MYSQL_HARNESS_PORT=43306 go run ./cmd/mysqlharness ./testdata/fixtures/schema.sql > /tmp/et-myrest/harness.log 2>&1 &)
for i in $(seq 1 60); do grep -q 'Press Ctrl' /tmp/et-myrest/harness.log && break; sleep 1; done
grep -q 'Press Ctrl' /tmp/et-myrest/harness.log || { echo "fixture load failed"; cat /tmp/et-myrest/harness.log; exit 1; }
(MYREST_LISTEN=127.0.0.1:3111 ./bin/myrest /tmp/et-myrest/myrest.conf > /tmp/et-myrest/server.log 2>&1 &)
sleep 2; head -1 /tmp/et-myrest/server.log
