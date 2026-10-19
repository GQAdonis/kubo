#!/bin/sh
# exact-probe.sh LABEL METHOD PATH TOKENFILE|- [ORIGIN]: prints label, status line only (no tokens, no bodies of secrets).
L=$1; M=$2; P=$3; T=$4; O=$5
[ "$O" = "-" ] && O=""
H=""; [ "$T" != "-" ] && H="Authorization: Bearer $(cat $T)"
OH=""; [ -n "$O" ] && OH="Origin: $O"
echo "## $L | $M $P | token=$T origin=${O:-none}"
if [ "$M" = POST ]; then
  wget -S -O /tmp/body --header "Content-Type: application/json" --header "Accept: application/json, text/event-stream" ${H:+--header "$H"} ${OH:+--header "$OH"} --post-data '{"jsonrpc":"2.0","id":1,"method":"initialize","params":{"protocolVersion":"2025-06-18","capabilities":{},"clientInfo":{"name":"probe","version":"0"}}}' "http://127.0.0.1:5050$P" 2>&1 | grep -E '^  HTTP/|^wget:' | head -2
else
  wget -S -O /tmp/body ${H:+--header "$H"} ${OH:+--header "$OH"} "http://127.0.0.1:5050$P" 2>&1 | grep -E '^  HTTP/|^wget:' | head -2
fi
