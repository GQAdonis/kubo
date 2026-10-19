#!/bin/sh
# run-scenario.sh NAME [GEN_TOKEN] [CFG]: start a container, wait for the listener, probe, remove it by ID.
# Host-side driver; probes run with docker exec. Tokens are never printed.
IMG=ghcr.io/gqadonis/kubo:1140ff10c
D=$(cd "$(dirname "$0")" && pwd)
N=$1; GEN=${2:-0}; CFG=$3
ARGS="-v $D/smoke-entry.sh:/smoke-entry.sh:ro -v $D/raw-run2.sh:/raw.sh:ro -v $D/tools.sh:/tools.sh:ro"
[ -n "$CFG" ] && ARGS="$ARGS -v $D/$CFG:/cfg.json:ro -e MCP_EXTRA=--mcp-config=/cfg.json"
[ "$GEN" = 1 ] && ARGS="$ARGS -e GEN_TOKEN=1"
ID=$(docker run -d --platform linux/amd64 --entrypoint sh $ARGS $IMG /smoke-entry.sh)
echo "$N=$ID" >> "$D/container-ids.txt"
i=0
until docker exec "$ID" sh -c 'nc -z 127.0.0.1 5151' 2>/dev/null; do
  i=$((i+1)); [ $i -gt 90 ] && { echo "listener-timeout"; break; }
  docker inspect -f '{{.State.Running}}' "$ID" | grep -q true || { echo "container-exited"; break; }
  sleep 1
done
echo "$ID"
