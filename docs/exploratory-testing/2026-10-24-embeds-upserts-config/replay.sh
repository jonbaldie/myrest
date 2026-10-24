#!/bin/sh
# Replay findings A-E from a fresh fixture load.
# Usage (from the repo root): replay.sh OUT
# Needs: MySQL 8.0 on 127.0.0.1:32891 (root password myrest), bin/myrest built,
# and no other process on ports 3124 and 3126.
set -u
E=docs/exploratory-testing/2026-10-24-embeds-upserts-config
OUT=$1
: > "$OUT"
export LOG="$OUT"
R="$E/req.sh"
J='Content-Type: application/json'
P='Prefer: return=representation'

pkill -f "bin/myrest $E/myrest.conf" 2>/dev/null; sleep 0.5
go run ./$E/reload 32891 testdata/fixtures/schema.sql >>"$OUT" 2>&1
docker exec -i et-myrest-20261024 mysql -uroot -pmyrest 2>/dev/null <<'SQL'
CREATE TABLE myrest_fixture.notes (
  id BIGINT UNSIGNED NOT NULL AUTO_INCREMENT PRIMARY KEY,
  title VARCHAR(255) NULL,
  body VARCHAR(255) NULL);
GRANT SELECT, INSERT, UPDATE, DELETE ON myrest_fixture.notes TO 'myrest_anon';
SQL
MYREST_LISTEN=127.0.0.1:3124 ./bin/myrest $E/myrest.conf >"$OUT.server" 2>&1 &
sleep 1.5

echo "## A. embed join type !inner / !left" >>"$OUT"
$R "A1 !inner" 'http://127.0.0.1:3124/items?select=id,orders!inner(id)&orders.id=eq.3'
$R "A2 !left" 'http://127.0.0.1:3124/items?select=id,orders!left(id)'
$R "A3 hint plus !inner" 'http://127.0.0.1:3124/deliveries?select=id,addresses!deliveries_from!inner(label)'
$R "A4 control: no join type" 'http://127.0.0.1:3124/items?select=id,orders(id)&orders.id=eq.3'

echo "## B. top-level filter on embed presence" >>"$OUT"
$R "B1 orders=not.is.null" 'http://127.0.0.1:3124/items?select=id,orders(id)&orders.id=eq.3&orders=not.is.null'

echo "## C. columns query parameter is ignored" >>"$OUT"
$R "C1 POST columns=title" -X POST -H "$J" -H "$P" -d '[{"title":"g","body":"should-be-ignored"}]' 'http://127.0.0.1:3124/notes?columns=title'
$R "C2 POST columns=title, extra key" -X POST -H "$J" -H "$P" -d '[{"title":"f","body":"ignored","extra":1}]' 'http://127.0.0.1:3124/notes?columns=title'
$R "C3 PATCH columns=title" -X PATCH -H "$J" -H "$P" -d '{"title":"h","body":"should-be-ignored-too"}' 'http://127.0.0.1:3124/notes?title=eq.g&columns=title'
$R "C4 read back" 'http://127.0.0.1:3124/notes?order=id.asc'

echo "## D. bulk POST with keys that do not match" >>"$OUT"
$R "D1 nullable table" -X POST -H "$J" -H "$P" -d '[{"title":"a"},{"body":"z"}]' 'http://127.0.0.1:3124/notes'
$R "D2 NOT NULL DEFAULT column" -X POST -H "$J" -H "$P" -d '[{"name":"pink"},{"name":"grey","tone":"mid"}]' 'http://127.0.0.1:3124/colors'
$R "D3 read back colors" 'http://127.0.0.1:3124/colors?order=id.asc'

pkill -f "bin/myrest $E/myrest.conf" 2>/dev/null; sleep 0.5

echo "## E. config text value without double quotes" >>"$OUT"
for c in single-quoted-schemas bare-inline-comment single-quotes; do
  echo >>"$OUT"; echo "### E $c.conf" >>"$OUT"; cat "$E/config/$c.conf" >>"$OUT"
  MYREST_LISTEN=127.0.0.1:3126 ./bin/myrest "$E/config/$c.conf" >"$OUT.e" 2>&1 &
  pid=$!; sleep 1.2
  BASE= $R "E $c GET /items" 'http://127.0.0.1:3126/items?limit=1'
  kill $pid 2>/dev/null; wait $pid 2>/dev/null
  echo "exit/log:" >>"$OUT"; sed 's/^[0-9/]* [0-9:]* /  > /' "$OUT.e" >>"$OUT"
done
rm -f "$OUT.e"
