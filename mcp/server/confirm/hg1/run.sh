#!/usr/bin/env bash
# One-command HG-1 run: starts a throwaway Kubo node (own IPFS_PATH, offline,
# non-default ports), runs attempt.sh against it, then stops that node.
# Your own node and ~/.ipfs are never used.
#
# Usage: mcp/server/confirm/hg1/run.sh [attempt.sh options]
set -u

here="$(cd "$(dirname "$0")" && pwd)"
repo_root="$(cd "$here/../../../.." && pwd)"
bin="$repo_root/cmd/ipfs/ipfs"
[ -x "$bin" ] || { echo "build the fork first: make build (in $repo_root)" >&2; exit 2; }
"$bin" mcp --help >/dev/null 2>&1 || { echo "$bin has no 'ipfs mcp'; run make build" >&2; exit 2; }

api_port="${HG1_API_PORT:-5193}"
gw_port="${HG1_GATEWAY_PORT:-8293}"
for p in "$api_port" "$gw_port"; do
	if lsof -nP -iTCP:"$p" -sTCP:LISTEN -t >/dev/null 2>&1; then
		echo "port $p is in use; set HG1_API_PORT / HG1_GATEWAY_PORT" >&2
		exit 2
	fi
done

work="$(mktemp -d)"
export IPFS_PATH="$work/repo"
export PATH="$repo_root/cmd/ipfs:$PATH"
daemon_pid=""
cleanup() {
	[ -n "$daemon_pid" ] && kill "$daemon_pid" 2>/dev/null && wait "$daemon_pid" 2>/dev/null
	rm -rf "$work"
}
trap cleanup EXIT

ipfs init --profile test -e >/dev/null
ipfs config Addresses.API "/ip4/127.0.0.1/tcp/$api_port"
ipfs config Addresses.Gateway "/ip4/127.0.0.1/tcp/$gw_port"
ipfs config --json Addresses.Swarm '[]'

ipfs daemon --offline >"$work/daemon.log" 2>&1 &
daemon_pid=$!
for _ in $(seq 1 30); do
	[ -f "$IPFS_PATH/api" ] && ipfs --api "/ip4/127.0.0.1/tcp/$api_port" version >/dev/null 2>&1 && break
	sleep 1
done
ipfs --api "/ip4/127.0.0.1/tcp/$api_port" version >/dev/null 2>&1 || { echo "throwaway node did not start:" >&2; cat "$work/daemon.log" >&2; exit 2; }
echo "throwaway node: $(ipfs version --commit) at /ip4/127.0.0.1/tcp/$api_port, IPFS_PATH=$IPFS_PATH"

bash "$here/attempt.sh" "$@"
