#!/bin/sh
# probe.sh METHOD PATH [BODY]: prints status line and body; bearer from /tmp/ui.token if present (never echoed).
M=$1; P=$2; B=$3
H=""
[ -s /tmp/ui.token ] && H="Authorization: Bearer $(cat /tmp/ui.token)"
if [ "$M" = POST ]; then
  wget -S -O- --header "Content-Type: application/json" ${H:+--header "$H"} --post-data "${B:-{\}}" "http://127.0.0.1:5151$P" 2>&1
else
  wget -S -O- ${H:+--header "$H"} "http://127.0.0.1:5151$P" 2>&1
fi
