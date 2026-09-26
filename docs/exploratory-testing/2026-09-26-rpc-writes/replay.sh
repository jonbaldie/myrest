#!/bin/bash
# Minimal reproducers for confirmed findings. Each block starts from reset.sh.
R=/tmp/et-myrest/req.sh; S=/tmp/et-myrest/reset.sh
echo "===== A: integer precision (write)"; $S
$R POST /items -H 'Content-Type: application/json' -H 'Prefer: return=representation' -d '{"id":9007199254740993,"name":"big"}'
$R GET '/items?id=eq.9007199254740993'
echo "===== A: integer precision (rpc)"
$R POST /rpc/add_them -H 'Content-Type: application/json' -d '{"a":9007199254740993,"b":0}'
$R GET '/rpc/add_them?a=9007199254740993&b=0'
echo "===== B: singular object on filtered rpc row set"; $S
$R GET '/rpc/list_items?id=eq.1'
$R GET '/rpc/list_items?id=eq.1' -H 'Accept: application/vnd.pgrst.object+json'
$R GET '/items?id=eq.1' -H 'Accept: application/vnd.pgrst.object+json'
echo "===== C: bulk POST mixed keys representation"; $S
$R POST /items -H 'Content-Type: application/json' -H 'Prefer: return=representation' -d '[{"name":"x"},{"name":"y","id":50}]'
$R GET '/items?name=in.(x,y)'
echo "===== D: PATCH changing primary key representation"; $S
$R POST /items -H "Content-Type: application/json" -d "{\"id\":10,\"name\":\"ten\"}"
$R PATCH "/items?id=eq.10" -H "Content-Type: application/json" -H "Prefer: return=representation" -d "{\"id\":20}"
$R GET "/items?id=in.(10,20)"
echo "===== E: JSON object argument to a JSON routine parameter (fixture addition as root)"; $S
docker exec -i et-myrest-mysql mysql -uroot -pmyrest myrest_fixture 2>&1 <<'SQL' | grep -v Warning
DELIMITER //
CREATE PROCEDURE et_echo_json(IN doc JSON, OUT back JSON) BEGIN SET back = doc; END//
DELIMITER ;
GRANT EXECUTE ON PROCEDURE myrest_fixture.et_echo_json TO 'myrest_anon';
SQL
pkill -USR1 -f 'bin/myrest /tmp/et-myrest/myrest.conf'; sleep 2
$R POST /rpc/et_echo_json -H 'Content-Type: application/json' -d '{"doc":{"k":1}}'
$R POST /rpc/et_echo_json -H 'Content-Type: application/json' -d '{"doc":"{\"k\":1}"}'
$R PATCH '/profiles?id=eq.1' -H 'Content-Type: application/json' -H 'Prefer: return=representation' -d '{"meta":{"k":1}}'
grep 'et_echo_json' /tmp/et-myrest/server.log
