#!/bin/bash
set -u
IPFS=/Users/gqadonis/Projects/references/kubo/cmd/ipfs/ipfs
W=$(mktemp -d)
export IPFS_PATH=$W/repo
freeport() { python3 -c 'import socket;s=socket.socket();s.bind(("127.0.0.1",0));print(s.getsockname()[1]);s.close()'; }
API=$(freeport); GW=$(freeport); SW=$(freeport); MCP=$(freeport)
echo "ports api=$API gw=$GW swarm=$SW mcp=$MCP workdir=$W"
$IPFS init --profile=test >/dev/null 2>&1
$IPFS config Addresses.API /ip4/127.0.0.1/tcp/$API
$IPFS config Addresses.Gateway /ip4/127.0.0.1/tcp/$GW
$IPFS config --json Addresses.Swarm "[\"/ip4/127.0.0.1/tcp/$SW\"]"
timeout 120 $IPFS daemon --offline >$W/daemon.log 2>&1 &
DPID=$!
for i in $(seq 1 40); do curl -s -X POST http://127.0.0.1:$API/api/v0/version >/dev/null && break; sleep 0.5; done
export IPFS_MCP_DESKTOP_UI_TOKEN=$(python3 -c 'import secrets;print(secrets.token_urlsafe(32))')
export IPFS_MCP_DESKTOP_ADMIN_TOKEN=$(python3 -c 'import secrets;print(secrets.token_urlsafe(32))')
mkdir -p $W/mcp
cat > $W/mcp/sidecar-config.json <<J
{"principals":[{"name":"desktop-ui","kind":"ui","credential":{"env":"IPFS_MCP_DESKTOP_UI_TOKEN"},"write_groups":["pin","content"],"egress_ack":["local"]},{"name":"desktop-admin","kind":"admin","credential":{"env":"IPFS_MCP_DESKTOP_ADMIN_TOKEN"}}],"agent":{"origin_allowlist":["webui://localhost"]}}
J
timeout 90 $IPFS mcp --http=127.0.0.1:$MCP --agui --confirm=native,desktop --settings-dir=$W/mcp --mcp-config=$W/mcp/sidecar-config.json >$W/mcp.out 2>$W/mcp.err &
MPID=$!
sleep 2; CPID=$(pgrep -P $MPID); echo "child pid $CPID"; ps -o pid,comm,args -p $CPID | cut -c1-120; curl -s -o /dev/null -w "mcp http status %{http_code}\n" http://127.0.0.1:$MCP/mcp
echo "daemon pid $DPID mcp pid $MPID"; 
for t in 5 15 30 45; do
  sleep $([ $t = 5 ] && echo 3 || echo 10 | sed 's/^/ /') 2>/dev/null
done >/dev/null
sleep 3
for n in 1 2 3 4 5; do
  echo "sample $n at $(date +%T): rss_kb=$(ps -o rss= -p $CPID | tr -d ' ')"
  sleep 6
done
echo "--- mcp stderr"; cat $W/mcp.err | head -20
kill -TERM $MPID 2>/dev/null; wait $MPID 2>/dev/null
kill -TERM $DPID 2>/dev/null; wait $DPID 2>/dev/null
rm -rf $W
