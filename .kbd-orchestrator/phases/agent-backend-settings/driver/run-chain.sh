#!/usr/bin/env bash
# Run a list of tasks through the ACP driver, recording each task with typed
# runtime transitions after the driver's checks pass. Stops at the first
# failure. Usage:
#   run-chain.sh <role> <timeout-s> <prompt-extra> <change>:<task> [...]
# Extra --add-dir arguments come from ADD_DIRS (space-separated).
set -u
here="$(cd "$(dirname "$0")" && pwd)"
kubo="$(cd "$here/../../../.." && pwd)"
role="$1"; to="$2"; extra="$3"; shift 3
export PATH="$HOME/.kimi-code/bin:$PATH"
cd "$kubo" || exit 2
add=()
for d in ${ADD_DIRS:-}; do add+=(--add-dir "$d"); done
uuid() { python3 -I -c 'import uuid;print(uuid.uuid4())'; }
record() {
  local ch="$1" t="$2"
  timeout 60 prometheus kbd task transition --command-id "$(uuid)" --phase agent-backend-settings \
    --change "$ch" --id "$t" --status in-progress </dev/null >/dev/null 2>&1
  if timeout 60 prometheus kbd task transition --command-id "$(uuid)" --phase agent-backend-settings \
    --change "$ch" --id "$t" --status complete \
    --summary "Completed by Kimi Code (ACP driver); driver checks passed; recorded by the lead" </dev/null >/dev/null 2>&1; then
    sed -i '' "s/^- \[ \] $t /- [x] $t /" "openspec/changes/$ch/tasks.md"
    echo "recorded $ch/$t"
  else
    echo "RECORD FAILED $ch/$t"
  fi
}
for item in "$@"; do
  ch="${item%%:*}"; t="${item##*:}"
  echo "=== $(date -u +%H:%M:%SZ) $ch/$t ($role)"
  timeout $((to + 300)) node "$here/kimi-acp.mjs" --change "$ch" --task "$t" --role "$role" \
    --timeout "$to" --answer-timeout 1500 "${add[@]}" --prompt-extra "$extra" </dev/null
  rc=$?
  echo "=== $ch/$t rc=$rc"
  [ "$rc" -eq 0 ] || { echo "STOP: $ch/$t failed (rc=$rc)"; exit "$rc"; }
  record "$ch" "$t"
done
echo "chain done: $role"
