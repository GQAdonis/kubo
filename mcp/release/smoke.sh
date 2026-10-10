#!/usr/bin/env bash
# Smoke test: run the host archive's `ipfs mcp` against a harness daemon.
# Requires dist/ from build.sh. Uses its own IPFS_PATH and non-default ports,
# and stops only the daemon it started (by PID). Never uses pkill/killall.
set -euo pipefail

HERE="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
DIST="$HERE/dist"
GOOS_H="$(go env GOOS)"; GOARCH_H="$(go env GOARCH)"
ARCHIVE="$(ls "$DIST"/kubo-mcp_*_"${GOOS_H}-${GOARCH_H}".tar.gz 2>/dev/null | head -1)"
[ -n "$ARCHIVE" ] || { echo "no host archive in $DIST; run build.sh" >&2; exit 1; }

SWARM_PORT="${SMOKE_SWARM_PORT:-14301}"
API_PORT="${SMOKE_API_PORT:-15301}"
GW_PORT="${SMOKE_GW_PORT:-18301}"
for p in "$SWARM_PORT" "$API_PORT" "$GW_PORT"; do
  case "$p" in 4000|4001|5001|8080) echo "refusing default port $p" >&2; exit 1;; esac
  if nc -z 127.0.0.1 "$p" 2>/dev/null; then echo "port $p is in use" >&2; exit 1; fi
done

WORK="$(mktemp -d "${TMPDIR:-/tmp}/kubo-mcp-smoke.XXXXXX")"
DAEMON_PID=""
cleanup() {
  if [ -n "$DAEMON_PID" ] && kill -0 "$DAEMON_PID" 2>/dev/null; then
    kill "$DAEMON_PID" 2>/dev/null || true
    for _ in 1 2 3 4 5 6 7 8 9 10; do kill -0 "$DAEMON_PID" 2>/dev/null || break; sleep 1; done
    kill -0 "$DAEMON_PID" 2>/dev/null && kill -9 "$DAEMON_PID" 2>/dev/null || true
  fi
  rm -rf "$WORK"
}
trap cleanup EXIT

tar -C "$WORK" -xzf "$ARCHIVE"
BIN="$(echo "$WORK"/kubo-mcp_*/ipfs)"
export IPFS_PATH="$WORK/repo"
"$BIN" init --profile=test >/dev/null 2>&1
"$BIN" config --json Addresses.Swarm "[\"/ip4/127.0.0.1/tcp/$SWARM_PORT\"]"
"$BIN" config Addresses.API "/ip4/127.0.0.1/tcp/$API_PORT"
"$BIN" config Addresses.Gateway "/ip4/127.0.0.1/tcp/$GW_PORT"

"$BIN" daemon --offline >"$WORK/daemon.log" 2>&1 &
DAEMON_PID=$!
echo "daemon pid $DAEMON_PID api $API_PORT swarm $SWARM_PORT gateway $GW_PORT repo $IPFS_PATH"
for _ in $(seq 1 60); do
  nc -z 127.0.0.1 "$API_PORT" 2>/dev/null && break
  kill -0 "$DAEMON_PID" 2>/dev/null || { cat "$WORK/daemon.log" >&2; echo "daemon exited" >&2; exit 1; }
  sleep 1
done
nc -z 127.0.0.1 "$API_PORT" || { cat "$WORK/daemon.log" >&2; echo "daemon API never came up" >&2; exit 1; }

python3 -I "$HERE/smoke_client.py" "$BIN" "/ip4/127.0.0.1/tcp/$API_PORT"
echo "SMOKE OK"
