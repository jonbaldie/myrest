#!/bin/sh
# Replay the confirmed failures from a reloaded fixture.
set -eu
DIR="$(cd "$(dirname "$0")" && pwd)"
ROOT="$(cd "$DIR/../../.." && pwd)"
cd "$ROOT"
PORT="${PORT:-32820}"
BASE="${BASE:-http://127.0.0.1:3111}"
CONF="$DIR/myrest.conf"
PIDFILE="$DIR/server.pid"

if [ -f "$PIDFILE" ]; then
  kill "$(cat "$PIDFILE")" 2>/dev/null || true
  sleep 0.3
fi

go run "$DIR/reload.go" "$PORT" "$ROOT/testdata/fixtures/schema.sql"

MYREST_LISTEN=127.0.0.1:3111 ./bin/myrest "$CONF" >> "$DIR/server.log" 2>&1 &
echo $! > "$PIDFILE"
sleep 0.5

echo "===== Content-Profile is absent on a two-database read"
curl -sS -D - -o /dev/null "$BASE/items?select=id&limit=1" | awk 'BEGIN{IGNORECASE=1} /^HTTP/ || /^Content-Profile:/ || /^content-profile:/ {print}'
echo "(no Content-Profile line means the header is absent)"

echo "===== Content-Profile is absent when Accept-Profile selects the second database"
curl -sS -D - -o /dev/null -H "Accept-Profile: myrest_hidden" "$BASE/outside_items?select=id" | awk 'BEGIN{IGNORECASE=1} /^HTTP/ || /^Content-Profile:/ || /^content-profile:/ {print}'

echo "===== OpenAPI before a same-name table: default OPTIONS versus document"
curl -sS -D - -o /dev/null -X OPTIONS "$BASE/items" | awk 'BEGIN{IGNORECASE=1} /^HTTP/ || /^Allow:/ {print}'

echo "===== add myrest_hidden.items with SELECT only, and a column-grant table"
docker exec et-myrest-20261010 mysql -uroot -pmyrest -e "
CREATE TABLE myrest_hidden.items (
  id BIGINT UNSIGNED NOT NULL AUTO_INCREMENT,
  name VARCHAR(255) NOT NULL,
  PRIMARY KEY (id)
);
INSERT INTO myrest_hidden.items (name) VALUES ('hidden-alpha');
GRANT SELECT ON myrest_hidden.items TO 'myrest_anon';
CREATE TABLE myrest_fixture.pay_slips (
  id BIGINT UNSIGNED NOT NULL AUTO_INCREMENT,
  employee VARCHAR(255) NOT NULL,
  salary INT NOT NULL,
  PRIMARY KEY (id)
);
INSERT INTO myrest_fixture.pay_slips (employee, salary) VALUES ('ada', 1000);
GRANT SELECT (id, employee) ON myrest_fixture.pay_slips TO 'myrest_anon';
"
kill -USR1 "$(cat "$PIDFILE")"
sleep 0.4

echo "===== OPTIONS default /items"
curl -sS -D - -o /dev/null -X OPTIONS "$BASE/items" | awk 'BEGIN{IGNORECASE=1} /^HTTP/ || /^Allow:/ {print}'
echo "===== OPTIONS hidden /items"
curl -sS -D - -o /dev/null -X OPTIONS -H "Accept-Profile: myrest_hidden" "$BASE/items" | awk 'BEGIN{IGNORECASE=1} /^HTTP/ || /^Allow:/ {print}'

python3 - << PY
import json, urllib.request
base = "$BASE"

def doc(profile=None):
    req = urllib.request.Request(base + "/")
    if profile:
        req.add_header("Accept-Profile", profile)
    with urllib.request.urlopen(req) as response:
        body = json.load(response)
        headers = {k.lower(): v for k, v in response.headers.items()}
    paths = body["paths"]
    print("profile", profile or "(none)", "Content-Profile", headers.get("content-profile"))
    print("  /items", paths.get("/items"))
    print("  has /outside_items", "/outside_items" in paths)
    print("  has /orders", "/orders" in paths)
    print("  path_count", len(paths))

doc()
doc("myrest_hidden")
PY

echo "===== column grant: API"
curl -sS -D - "$BASE/pay_slips?select=id,employee"
echo
echo "===== column grant: MySQL as authenticator after SET ROLE"
docker exec et-myrest-20261010 mysql -uauthenticator -psecret -N -e "SET ROLE myrest_anon; SELECT id, employee FROM myrest_fixture.pay_slips;"
