#!/usr/bin/env bash
# Run driver tasks in order; stop at the first failure.
# Usage: run-batch.sh <role> <change>:<task> [<change>:<task> ...]
set -u
here="$(cd "$(dirname "$0")" && pwd)"
role="$1"; shift
for item in "$@"; do
  change="${item%%:*}"; task="${item##*:}"
  echo "=== $(date -u +%H:%M:%SZ) $change/$task ($role)"
  timeout 2000 node "$here/kimi-task.mjs" --change "$change" --task "$task" --role "$role" \
    --add-dir /Users/gqadonis/Projects/references/ipfs-webui \
    --add-dir /Users/gqadonis/Projects/references/ipfs-desktop \
    --timeout 1800 --kbd </dev/null
  rc=$?
  echo "=== $change/$task rc=$rc"
  [ "$rc" -eq 0 ] || { echo "STOP: $change/$task failed (rc=$rc)"; exit "$rc"; }
done
echo "=== batch complete"
