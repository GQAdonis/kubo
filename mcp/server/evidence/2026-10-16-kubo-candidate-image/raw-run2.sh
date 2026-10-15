#!/bin/sh
# raw.sh METHOD PATH [BODY]: raw HTTP over nc so error bodies are visible; bearer from /tmp/ui.token (never printed).
M=$1; P=$2; B=${3:-}
TOK=$(cat /tmp/ui.token 2>/dev/null)
L=$(printf %s "$B" | wc -c)
{
  printf '%s %s HTTP/1.1\r\nHost: 127.0.0.1:5151\r\nConnection: close\r\n' "$M" "$P"
  [ -n "$TOK" ] && printf 'Authorization: Bearer %s\r\n' "$TOK"
  [ -n "$B" ] && printf 'Content-Type: application/json\r\nContent-Length: %s\r\n' "$L"
  printf '\r\n%s' "$B"
sleep 3; } | nc -w 6 127.0.0.1 5151
