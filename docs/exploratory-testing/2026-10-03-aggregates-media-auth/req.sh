#!/bin/bash
m=$1; p=$2; shift 2
out=$(curl -s -i -X "$m" "http://127.0.0.1:3111$p" "$@" 2>&1)
{ echo "### $m $p $*"; echo "$out"; echo; } >> ${EVIDENCE_LOG:-/tmp/et-myrest/log.txt}
echo "### $m $p $*"
echo "$out" | grep -vE '^(Date|Content-Length):'
echo
