#!/usr/bin/env bash
# HG-1 UI case for the native confirmation prompt (task 1.5). Operator-run.
#
# While a REAL prompt is pending, a background process under the same user
# (nohup, no TTY) tries synthetic input:
#   macOS:        osascript System Events click and keystroke Return
#   Linux X11:    xdotool key Return and a click
#   Linux Wayland: recorded as unavailable (no synthetic input tool applies)
# Pass condition: the write is NOT performed, the prompt still asks for Touch
# ID or the password, and the call is later reported denied or expired.
#
# Needs: a running Kubo daemon (own IPFS_PATH, non-default ports), `ipfs` on
# PATH built with this change, a graphical session, and on Linux the polkit
# action installed (polkit/INSTALL.md). The first run on macOS asks you to
# allow your terminal under System Settings > Privacy & Security >
# Accessibility; grant it, because the point is to show that even a process
# that may click still cannot supply the presence factor.
#
# Usage: attempt.sh [--wait SECONDS]   (default 20, time to leave the prompt up)
set -u

WAIT=20
if [ "${1:-}" = "--wait" ]; then WAIT="${2:-20}"; fi

here="$(cd "$(dirname "$0")" && pwd)"
evid="$here/../evidence"
mkdir -p "$evid"
os="$(uname -s)"
stamp="$(date -u +%Y%m%dT%H%M%SZ)"
log="$evid/hg1-$os-$stamp.txt"
work="$(mktemp -d)"
trap 'kill "${srv:-0}" 2>/dev/null; rm -rf "$work"' EXIT

say() { printf '%s\n' "$*" | tee -a "$log"; }

say "HG-1 UI attempt: $os $(uname -r) $(date -u +%FT%TZ)"
say "session: ${XDG_SESSION_TYPE:-n/a}"

case "$os" in
Darwin) mech=macos-localauth ;;
Linux) mech=linux-polkit ;;
*) say "RESULT: $os has no native prompt; confirmation is refused (fail closed)"; exit 0 ;;
esac

if [ "$os" = Linux ] && [ "${XDG_SESSION_TYPE:-}" = wayland ]; then
	say "Wayland: synthetic input from a background process is not available;"
	say "RESULT: unavailable (recorded). X11 case needs an X11 session."
	exit 0
fi

command -v ipfs >/dev/null || { say "ipfs not on PATH"; exit 2; }
ipfs id >/dev/null 2>&1 || { say "no reachable daemon (set IPFS_PATH and start 'ipfs daemon')"; exit 2; }

cid="$(printf 'hg1 attempt %s' "$stamp" | ipfs add -Q --pin=false --cid-version=1)"
say "target CID: $cid (not pinned)"

# A client that cannot elicit: no capabilities. Its pin_add opens a native prompt.
mkfifo "$work/in"
ipfs mcp --confirm=native <"$work/in" >"$work/out" 2>"$work/err" &
srv=$!
exec 3>"$work/in"
printf '%s\n' '{"jsonrpc":"2.0","id":1,"method":"initialize","params":{"protocolVersion":"2025-06-18","capabilities":{},"clientInfo":{"name":"hg1","version":"0"}}}' >&3
printf '%s\n' '{"jsonrpc":"2.0","method":"notifications/initialized"}' >&3
call() {
	printf '%s\n' "{\"jsonrpc\":\"2.0\",\"id\":$1,\"method\":\"tools/call\",\"params\":{\"name\":\"pin_add\",\"arguments\":{\"path\":\"/ipfs/$cid\"}}}" >&3
}
call 2

say "waiting for the real prompt (the macOS helper can take ~6 s to start)..."
sleep 12
if ! grep -q confirmation_required "$work/out"; then
	say "RESULT: no prompt was opened (server output follows)"; cat "$work/out" "$work/err" | tee -a "$log"; exit 2
fi
say "prompt pending; starting same-user background attempts (nohup, no TTY)"

attempts="$work/attempts.sh"
case "$os" in
Darwin)
	cat >"$attempts" <<'EOS'
echo "[osascript] keystroke Return"
osascript -e 'tell application "System Events" to key code 36' 2>&1
echo "[osascript] click default button of the front window of every process"
osascript -e 'tell application "System Events" to repeat with p in (processes whose visible is true)
try
click button 1 of window 1 of p
end try
end repeat' 2>&1
echo "[osascript] type a guessed password"
osascript -e 'tell application "System Events" to keystroke "password"' -e 'tell application "System Events" to key code 36' 2>&1
echo "attempts done"
EOS
	;;
Linux)
	command -v xdotool >/dev/null || { say "xdotool missing; cannot attempt on X11"; exit 2; }
	cat >"$attempts" <<'EOS'
echo "[xdotool] key Return"; xdotool key Return 2>&1
echo "[xdotool] click"; xdotool click 1 2>&1
echo "[xdotool] type a guessed password"; xdotool type --delay 20 password 2>&1; xdotool key Return 2>&1
echo "attempts done"
EOS
	;;
esac
nohup bash "$attempts" </dev/null >"$work/attempt.log" 2>&1 &
wait $! 2>/dev/null
sleep 3
say "--- attempt output ---"
tee -a "$log" <"$work/attempt.log"
say "--- end ---"

if ipfs pin ls --type=recursive "$cid" >/dev/null 2>&1; then
	say "FAIL: $cid was pinned while the prompt was pending (mechanism $mech must be disabled in verdicts.go)"
	exit 1
fi
say "check 1 passed: $cid is not pinned after the synthetic input"

printf '\nIs the Touch ID / password / polkit prompt STILL on screen? [y/N] '
read -r on_screen
say "operator: prompt still on screen = ${on_screen:-n}"
say "Now CANCEL the prompt yourself (or wait $WAIT s). Do not approve it."
sleep "$WAIT"

call 3
sleep 4
if ipfs pin ls --type=recursive "$cid" >/dev/null 2>&1; then
	say "FAIL: $cid is pinned"; exit 1
fi
say "check 2 passed: $cid is still not pinned after the prompt ended"
say "repeat-call result:"
grep -o '"code":"[a-z_]*"' "$work/out" | sort | uniq -c | tee -a "$log"

say "RESULT: PASS for $mech (record the platform limits in MECHANISMS.md)"
say "Evidence file: $log"
