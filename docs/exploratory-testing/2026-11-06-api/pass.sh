#!/usr/bin/env bash
set -Eeuo pipefail
cd "$(git -C "$(dirname "$0")" rev-parse --show-toplevel)"
mkdir -p .run
export GOMODCACHE="$PWD/.run/mod" GOCACHE="$PWD/.run/cache" GOPATH="$PWD/.run/gopath"
go version
go mod download -modcacherw
go mod verify
go build -mod=readonly -o .run/myrest ./cmd/myrest
if ss -ltnH | awk '{print $4}' | grep -qE '(^|:)33176$'; then echo 'port 33176 is in use' >&2; exit 1; fi
name=et-myrest-20261106-$$
server_pid=
cleanup() {
  set +e
  if [ -n "$server_pid" ]; then kill "$server_pid" 2>/dev/null; wait "$server_pid" 2>/dev/null; fi
  volumes=$(docker inspect "$name" --format '{{range .Mounts}}{{if eq .Type "volume"}}{{.Name}} {{end}}{{end}}' 2>/dev/null)
  echo "cleanup container=$name server_pid=${server_pid:-none} volumes=$volumes"
  docker rm -fv "$name" 2>/dev/null
  docker ps -a --filter "name=^/${name}$" --format '{{.Names}} {{.Status}}'
  for volume in $volumes; do if docker volume inspect "$volume" >/dev/null 2>&1; then echo "VOLUME STILL EXISTS $volume"; else echo "volume removed $volume"; fi; done
  if [ -n "$server_pid" ] && kill -0 "$server_pid" 2>/dev/null; then echo 'SERVER STILL ALIVE'; else echo 'server stopped'; fi
}
trap cleanup EXIT
trap 'exit 130' INT
trap 'exit 143' TERM
echo "registered container=$name; service port=127.0.0.1:33176; teardown=docker-rm-and-kill"
docker run -d --name "$name" -e MYSQL_ROOT_PASSWORD=myrest -e MYSQL_USER=myrest -e MYSQL_PASSWORD=myrest -p 127.0.0.1::3306 mysql:8.0 --default-authentication-plugin=mysql_native_password
port=$(docker port "$name" 3306/tcp | awk -F: '{print $NF}')
echo "mysql port=$port"
for i in $(seq 1 90); do if docker exec "$name" mysqladmin ping -h 127.0.0.1 -uroot -pmyrest --silent >/dev/null 2>&1; then break; fi; sleep 1; done
docker exec "$name" mysqladmin ping -h 127.0.0.1 -uroot -pmyrest --silent
reload() { go run -mod=readonly ./docs/exploratory-testing/2026-10-24-embeds-upserts-config/reload "$port" testdata/fixtures/schema.sql; }
reload
printf 'db-uri = "mysql://authenticator:secret@127.0.0.1:%s/"\ndb-schemas = "myrest_fixture"\ndb-anon-role = "myrest_anon"\ndb-tx-end = "commit-allow-override"\n' "$port" > .run/myrest.conf
MYREST_LISTEN=127.0.0.1:33176 .run/myrest .run/myrest.conf > .run/server.log 2>&1 & server_pid=$!
echo "registered server pid=$server_pid"
ready=0
for i in $(seq 1 60); do
  if ! kill -0 "$server_pid" 2>/dev/null; then read -r -d '' _ < .run/server.log || true; printf '%s\n' "$_"; exit 1; fi
  if grep -q 'listening' .run/server.log && curl -sf 'http://127.0.0.1:33176/items?limit=1' >/dev/null; then ready=1; break; fi
  sleep 1
done
if [ "$ready" -ne 1 ]; then echo 'service did not become ready' >&2; exit 1; fi
base=http://127.0.0.1:33176
request() { echo; printf '>>> '; printf '%q ' "$@"; echo; curl -sS -i --max-time 15 "$@"; echo; }
request "$base/items?select=id,name&order=id.asc&limit=1"
request -H 'Range: 1-1' -H 'Prefer: count=exact' "$base/items?select=id,name&order=id.asc"
request "$base/items?select=id,name,orders(id)&orders.order=id.desc&order=id.asc"
request -H 'Content-Type: application/json' -H 'Prefer: return=representation' -d '{"id":9001,"name":"explore"}' "$base/items"
request -X PATCH -H 'Content-Type: application/json' -H 'Prefer: return=representation' -d '{"name":"changed"}' "$base/items?id=eq.9001"
request "$base/items?id=eq.9001&select=id,name"
request -X DELETE -H 'Prefer: return=representation' "$base/items?id=eq.9001"
request "$base/items?id=eq.9001"
request -H 'Content-Type: application/json' -H 'Prefer: return=representation,tx=rollback' -d '{"id":9002,"name":"rollback"}' "$base/items"
request "$base/items?id=eq.9002"
request -H 'Content-Type: application/json' -H 'Prefer: return=representation,tx=commit' -d '{"id":9003,"name":"commit"}' "$base/items"
request "$base/items?id=eq.9003"
request -H 'Content-Type: application/json' -d '{"a":1,"b":2}' "$base/rpc/add_them"
request "$base/rpc/add_them?a=4&b=5"
request -H 'Content-Type: application/json' -d '{"a":1,"b":2,"extra":3}' "$base/rpc/add_them"
request -H 'Content-Type: application/json' -X POST "$base/rpc/write_marker" -H 'Prefer: tx=rollback'
request "$base/addresses?label=eq.rpc-write"
request -H 'Content-Type: application/json' -X POST "$base/rpc/write_marker" -H 'Prefer: tx=commit'
request "$base/addresses?label=eq.rpc-write"
request -H 'Content-Type: application/json' -H 'Prefer: return=representation' -d '[{"id":9010,"name":"a"},{"id":9011}]' "$base/items"
request "$base/items?id=in.(9010,9011)&select=id,name"
request -H 'Content-Type: application/json' -d '{"id":9012,"name":"forbidden"}' "$base/items?columns=id"
request "$base/items?id=eq.9012"
request "$base/items?select=id,name&order=id.asc&limit=1&offset=1"
request "$base/rpc/add_them?a=4&b=5&extra=3"
echo 'END OF JOURNEYS'
