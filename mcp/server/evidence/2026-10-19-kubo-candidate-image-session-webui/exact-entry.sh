#!/bin/sh
# Container-local entrypoint mirroring StatefulSet ipfs: daemon (offline) + the ipfs-mcp sidecar command verbatim.
# Same netns for both (as in the pod), so API 5001 and MCP 5050 are the manifest's values; nothing is published to the host.
export IPFS_PATH=/tmp/ipfs-exact
IPFS_MCP_TOKEN=$(head -c 24 /dev/urandom | od -An -tx1 | tr -d ' \n')
export IPFS_MCP_TOKEN
printf %s "$IPFS_MCP_TOKEN" >/tmp/mcp.token
ipfs init --profile=test >/tmp/init.log 2>&1
ipfs config --json Addresses.Swarm '["/ip4/127.0.0.1/tcp/4101"]'
ipfs config Addresses.API /ip4/127.0.0.1/tcp/5001
ipfs config Addresses.Gateway /ip4/127.0.0.1/tcp/8080
ipfs daemon --offline >/tmp/daemon.log 2>&1 &
i=0
until ipfs --api=/ip4/127.0.0.1/tcp/5001 id >/dev/null 2>&1; do
  i=$((i+1)); [ "$i" -gt 60 ] && { echo daemon-timeout; exit 1; }
  sleep 1
done
exec /usr/local/bin/ipfs mcp --http=0.0.0.0:5050 --api=/ip4/127.0.0.1/tcp/5001 --auth-token-env=IPFS_MCP_TOKEN --mcp-config=/mcp-config/config.json >/tmp/mcp.log 2>&1
