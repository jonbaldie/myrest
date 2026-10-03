#!/bin/bash
DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
mkdir -p /tmp/et-myrest
cp "$DIR/myrest.conf" /tmp/et-myrest/myrest.conf
cp "$DIR/req.sh" /tmp/et-myrest/req.sh
cp "$DIR/reset.sh" /tmp/et-myrest/reset.sh
R=/tmp/et-myrest/req.sh
S=/tmp/et-myrest/reset.sh

echo "===== A: many-to-many embed with count() aggregate"
$S
$R GET "/items?select=id,tags(count())"
$R GET "/tags?select=id,items(count())"

echo "===== A: many-to-many embed with sum() aggregate"
$R GET "/tags?select=id,items(id.sum())"

echo "===== Comparison: one-to-many embed with count() aggregate"
$R GET "/items?select=name,orders(count())&order=name.asc"
