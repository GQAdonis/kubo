#!/usr/bin/env bash
# Boundary board for the backend and UI lanes; see README.md in this folder.
#
# Usage:
#   board.sh post --from backend/<role> --to ui --kind request --subject "..." \
#                 [--paths a,b] [--refs x,y] [--reply-to <id>] --body "..."
#   board.sh open [backend|ui]     # open items addressed to that lane
#   board.sh claims                # live claims (claim with no later release)
#   board.sh tail [N]              # last N messages, one line each
#
# Requires jq. Exit 0 on success, 1 on usage or validation errors.
set -euo pipefail

DIR="$(cd "$(dirname "$0")" && pwd)"
LOG="$DIR/log.jsonl"
KINDS="claim release request contract-change handoff ack blocker note"
touch "$LOG"

die() { echo "board.sh: $*" >&2; exit 1; }
csv_json() { if [ -z "${1:-}" ]; then echo '[]'; else jq -cn --arg s "$1" '$s | split(",") | map(select(length > 0))'; fi; }

cmd="${1:-}"; shift || true
case "$cmd" in
  post)
    from= to= kind= subject= body= paths= refs= reply=
    while [ $# -gt 0 ]; do
      case "$1" in
        --from) from="$2"; shift 2 ;;
        --to) to="$2"; shift 2 ;;
        --kind) kind="$2"; shift 2 ;;
        --subject) subject="$2"; shift 2 ;;
        --body) body="$2"; shift 2 ;;
        --paths) paths="$2"; shift 2 ;;
        --refs) refs="$2"; shift 2 ;;
        --reply-to) reply="$2"; shift 2 ;;
        *) die "unknown argument $1" ;;
      esac
    done
    [[ "$from" =~ ^(backend|ui)/[a-z0-9-]+$ ]] || die "--from must be backend/<role> or ui/<role>"
    [[ "$to" =~ ^(backend|ui|all)(/[a-z0-9-]+)?$ ]] || die "--to must be backend, ui, all or <lane>/<role>"
    [[ " $KINDS " == *" $kind "* ]] || die "--kind must be one of: $KINDS"
    [ -n "$subject" ] && [ -n "$body" ] || die "--subject and --body are required"
    lane="${from%%/*}"
    id="$lane-$(date -u +%Y%m%dT%H%M%SZ)-$(od -An -N3 -tx1 /dev/urandom | tr -d ' \n')"
    line=$(jq -cn --arg id "$id" --arg ts "$(date -u +%Y-%m-%dT%H:%M:%SZ)" \
      --arg from "$from" --arg to "$to" --arg kind "$kind" --arg subject "$subject" \
      --arg body "$body" --argjson paths "$(csv_json "$paths")" --argjson refs "$(csv_json "$refs")" \
      --arg reply "$reply" \
      '{id:$id, ts:$ts, from:$from, to:$to, kind:$kind, subject:$subject, paths:$paths, refs:$refs, body:$body, reply_to:(if $reply == "" then null else $reply end)}')
    # One write of one line with O_APPEND is atomic for lines under PIPE_BUF;
    # longer lines take a mkdir lock so two writers never interleave.
    lock="$DIR/.log.lock"
    for _ in $(seq 50); do mkdir "$lock" 2>/dev/null && break; sleep 0.1; done
    [ -d "$lock" ] || die "could not take $lock (another writer holds it)"
    printf '%s\n' "$line" >> "$LOG"; rmdir "$lock"
    echo "$id"
    ;;
  open)
    lane="${1:-}"; [[ "$lane" =~ ^(backend|ui)$ ]] || die "open needs backend or ui"
    # Items addressed to the lane (or all) of kind request/blocker/contract-change/handoff/claim
    # that no later message answers with reply_to.
    jq -s -r --arg lane "$lane" '
      (map(.reply_to) | map(select(. != null))) as $answered
      | .[] | select((.to == $lane or (.to | startswith($lane + "/")) or .to == "all")
                    and (.kind | IN("request","blocker","contract-change","handoff","claim"))
                    and ((.id | IN($answered[])) | not))
      | "\(.ts)  \(.id)  \(.kind)  from \(.from): \(.subject)"' "$LOG"
    ;;
  claims)
    jq -s -r '
      reduce .[] as $m ({}; if $m.kind == "claim" then .[$m.from + "|" + ($m.paths|join(","))] = $m
                            elif $m.kind == "release" then with_entries(select(.value.from != $m.from or ((.value.paths - $m.paths) | length) > 0))
                            else . end)
      | .[] | "\(.ts)  \(.from)  claims \(.paths | join(", "))  (\(.id))"' "$LOG"
    ;;
  tail)
    n="${1:-20}"
    tail -n "$n" "$LOG" | jq -r '"\(.ts)  \(.kind)  \(.from) -> \(.to): \(.subject)"'
    ;;
  *) sed -n '2,12p' "$0"; exit 1 ;;
esac
