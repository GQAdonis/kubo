#!/usr/bin/env bash
# Load the IPFS plugin in each installed harness without touching the
# operator's own configuration, and record the outputs.
#
#   mcp/plugin/verify.sh --ipfs-bin /path/to/ipfs [claude] [codex] [opencode]
#
# Everything runs with HOME and each harness's config/data directories pointed
# at a scratch directory (mktemp -d). A private Kubo repo and daemon are
# started on non-default ports (swarm 4301, API 5301, gateway 8381; override
# with VERIFY_SWARM_PORT, VERIFY_API_PORT, VERIFY_GATEWAY_PORT) and only that
# daemon's PID is stopped on exit. No harness login is used or needed.
#
# Output goes to mcp/plugin/evidence/<harness>-isolated-load.txt.
# Limits: Claude Code cannot run a model session without the operator's login,
# so its check stops at validate, install, details and the MCP health check.
set -euo pipefail

repo="$(cd "$(dirname "$0")/../.." && pwd)"
evidence="$repo/mcp/plugin/evidence"
ipfs_bin=""
targets=()
while [ $# -gt 0 ]; do
  case "$1" in
    --ipfs-bin) ipfs_bin="$2"; shift 2 ;;
    claude|codex|opencode) targets+=("$1"); shift ;;
    *) echo "unknown argument: $1" >&2; exit 2 ;;
  esac
done
[ -n "$ipfs_bin" ] && [ -x "$ipfs_bin" ] || { echo "--ipfs-bin must name an executable ipfs binary" >&2; exit 2; }
[ "${#targets[@]}" -gt 0 ] || targets=(claude codex opencode)

work="$(mktemp -d)"
mkdir -p "$work/bin" "$work/home" "$evidence"
cp "$ipfs_bin" "$work/bin/ipfs"
ln -s "$work/repo" "$work/home/.ipfs" # default repo location for the fake HOME
daemon_pid=""
cleanup() {
  if [ -n "$daemon_pid" ]; then kill "$daemon_pid" 2>/dev/null || true; fi
  rm -rf "$work"
}
trap cleanup EXIT

export PATH="$work/bin:$PATH" IPFS_TELEMETRY=off DO_NOT_TRACK=1
swarm="${VERIFY_SWARM_PORT:-4301}"; api="${VERIFY_API_PORT:-5301}"; gw="${VERIFY_GATEWAY_PORT:-8381}"
for port in "$swarm" "$api" "$gw"; do
  case "$port" in 4001|5001|8080|4000) echo "refusing default port $port" >&2; exit 2 ;; esac
done

# Private repo and daemon, offline, loopback only.
IPFS_PATH="$work/repo" ipfs init --profile=test >/dev/null 2>&1
IPFS_PATH="$work/repo" ipfs config --json Addresses.Swarm "[\"/ip4/127.0.0.1/tcp/$swarm\"]"
IPFS_PATH="$work/repo" ipfs config Addresses.API "/ip4/127.0.0.1/tcp/$api"
IPFS_PATH="$work/repo" ipfs config Addresses.Gateway "/ip4/127.0.0.1/tcp/$gw"
IPFS_PATH="$work/repo" ipfs daemon --offline >"$work/daemon.log" 2>&1 &
daemon_pid=$!
for _ in $(seq 1 40); do
  if grep -q "Daemon is ready" "$work/daemon.log" 2>/dev/null; then break; fi
  sleep 0.5
done
grep -q "Daemon is ready" "$work/daemon.log" || { echo "private daemon did not start" >&2; exit 1; }
echo "private daemon pid=$daemon_pid api=$api"

export HOME="$work/home"
mkdir -p "$work/scratch-cwd"

verify_claude() {
  set +e # record every command's output even when one fails
  command -v claude >/dev/null || { echo "claude not installed" >&2; return 1; }
  export CLAUDE_CONFIG_DIR="$work/claude-cfg"
  mkdir -p "$CLAUDE_CONFIG_DIR"
  {
    echo "# claude $(claude --version)"
    echo "\$ claude plugin validate mcp/plugin/ipfs"
    (cd "$repo" && claude plugin validate mcp/plugin/ipfs) 2>&1
    echo "\$ claude plugin validate ."
    (cd "$repo" && claude plugin validate .) 2>&1
    echo "\$ claude plugin marketplace add <repo>"
    (cd "$repo" && claude plugin marketplace add "$repo") 2>&1
    echo "\$ claude plugin install ipfs@kubo-ipfs"
    claude plugin install ipfs@kubo-ipfs 2>&1
    echo "\$ claude plugin list"
    claude plugin list 2>&1
    echo "\$ claude plugin details ipfs"
    claude plugin details ipfs 2>&1
    echo "\$ claude mcp list   (health check starts 'ipfs mcp' against the private daemon)"
    (cd "$work/scratch-cwd" && timeout 90 claude mcp list) 2>&1
    echo "\$ claude auth status   (a model session needs the operator's login; not used)"
    claude auth status 2>&1 | sed -n 1,3p
  } >"$evidence/claude-isolated-load.txt"
  unset CLAUDE_CONFIG_DIR
  set -e
}

verify_codex() {
  set +e # record every command's output even when one fails
  command -v codex >/dev/null || { echo "codex not installed" >&2; return 1; }
  export CODEX_HOME="$work/codex-home"
  mkdir -p "$CODEX_HOME"
  {
    echo "# $(codex --version)"
    echo "\$ codex plugin marketplace add <repo>"
    codex plugin marketplace add "$repo" 2>&1
    echo "\$ codex plugin marketplace list"
    codex plugin marketplace list 2>&1
    echo "\$ codex plugin add ipfs@kubo-ipfs"
    codex plugin add ipfs@kubo-ipfs 2>&1
    echo "\$ codex plugin list"
    codex plugin list 2>&1
    echo "\$ codex mcp list"
    codex mcp list 2>&1
    echo "\$ codex_probe.py (app-server: plugin/read, skills/list, mcpServerStatus/list)"
    timeout 150 python3 -I "$repo/mcp/plugin/tools/codex_probe.py" "$repo" 2>&1
  } >"$evidence/codex-isolated-load.txt"
  unset CODEX_HOME
  set -e
}

verify_opencode() {
  set +e # record every command's output even when one fails
  command -v opencode >/dev/null || { echo "opencode not installed" >&2; return 1; }
  export XDG_CONFIG_HOME="$HOME/.config" XDG_DATA_HOME="$HOME/.local/share" \
         XDG_CACHE_HOME="$HOME/.cache" XDG_STATE_HOME="$HOME/.local/state" \
         OPENCODE_DISABLE_AUTOUPDATE=1
  mkdir -p "$XDG_CONFIG_HOME"
  local plugin_url="file://$repo/mcp/plugin/ipfs/opencode/ipfs-mcp.js"
  printf '{ "$schema": "https://opencode.ai/config.json", "plugin": ["%s"] }\n' "$plugin_url" >"$work/opencode-plugin.json"
  {
    echo "# opencode $(opencode --version)"
    echo "# config: only a plugin entry; the plugin adds the MCP server and the skills path"
    cat "$work/opencode-plugin.json"
    export OPENCODE_CONFIG="$work/opencode-plugin.json"
    cd "$work/scratch-cwd"
    echo "\$ opencode debug paths   (all under the scratch HOME)"
    opencode debug paths 2>&1
    echo "\$ opencode debug config"
    timeout 60 opencode debug config 2>&1
    echo "\$ opencode debug skill   (names)"
    timeout 60 opencode debug skill 2>&1 | python3 -I -c 'import sys,json; print([s["name"] for s in json.load(sys.stdin)])'
    echo "\$ opencode mcp list   (starts 'ipfs mcp' against the private daemon)"
    timeout 60 opencode mcp list 2>&1 | sed 's/\x1b\[[0-9;]*m//g'
  } >"$evidence/opencode-isolated-load.txt"
  set -e
}

for t in "${targets[@]}"; do
  echo "== $t"
  "verify_$t"
  echo "wrote $evidence/$t-isolated-load.txt"
done
