#!/bin/sh
# Container-local smoke entrypoint: own IPFS_PATH, non-default ports only.
# MCP_EXTRA carries scenario-specific ipfs mcp flags (for example --mcp-config).
# GEN_TOKEN=1 creates a throwaway ui token inside the container only (never printed).
export IPFS_PATH=/tmp/ipfs-smoke
if [ "$GEN_TOKEN" = "1" ]; then
  IPFS_MCP_UI_TOKEN=$(head -c 24 /dev/urandom | od -An -tx1 | tr -d ' \n')
  export IPFS_MCP_UI_TOKEN
  printf %s "$IPFS_MCP_UI_TOKEN" >/tmp/ui.token
fi
ipfs init --profile=test >/tmp/init.log 2>&1
ipfs config Addresses.API /ip4/127.0.0.1/tcp/5101
ipfs config Addresses.Gateway /ip4/127.0.0.1/tcp/8181
ipfs config --json Addresses.Swarm '["/ip4/127.0.0.1/tcp/4101"]'
ipfs daemon --offline >/tmp/daemon.log 2>&1 &
i=0
until ipfs --api=/ip4/127.0.0.1/tcp/5101 id >/dev/null 2>&1; do
  i=$((i+1))
  if [ "$i" -gt 60 ]; then echo daemon-timeout; exit 1; fi
  sleep 1
done
exec ipfs mcp --api=/ip4/127.0.0.1/tcp/5101 --http=127.0.0.1:5151 --agui $MCP_EXTRA >/tmp/mcp.log 2>&1
