#!/bin/sh
# raw-oidc.sh METHOD PATH BODY TOKFILE ORIGIN: raw HTTP over nc; the bearer comes from TOKFILE ("-" for none) and is never printed.
M=$1; P=$2; B=$3; TF=$4; O=$5
L=$(printf %s "$B" | wc -c)
{
  printf '%s %s HTTP/1.1\r\nHost: 127.0.0.1:5151\r\nConnection: close\r\n' "$M" "$P"
  [ "$TF" != "-" ] && printf 'Authorization: Bearer %s\r\n' "$(cat "$TF")"
  [ -n "$O" ] && printf 'Origin: %s\r\n' "$O"
  [ -n "$B" ] && printf 'Content-Type: application/json\r\nContent-Length: %s\r\n' "$L"
  printf '\r\n%s' "$B"
sleep 2; } | nc -w 5 127.0.0.1 5151
