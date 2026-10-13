#!/usr/bin/env bash
# HG-1 UI case for the native confirmation prompt (task 1.5). Operator-run.
#
# While a REAL prompt is pending, a background process under the same user
# (nohup, no TTY) tries synthetic input, aimed ONLY at the authentication UI:
#   macOS:        osascript System Events, click and keystroke Return, sent only
#                 to the LocalAuthentication / security agent processes below
#   Linux X11:    xdotool key / click / type, sent with --window to the polkit
#                 agent window found by name
#   Linux Wayland: recorded as unavailable (no synthetic input tool applies)
# Nothing is ever sent to another application: the script never iterates over
# processes or windows to send input, and it types only when the frontmost
# process is one of the authentication agents.
#
# Result lines (the last one starting with "RESULT:"):
#   PASS          an auth window was present, input was delivered to it, the
#                 write was not performed and the window was still there
#   INCONCLUSIVE  no auth window was observed, no input could be delivered, or
#                 the window was gone after the attempts; proves nothing
#   FAIL          the write was performed
#
# Needs: a running Kubo daemon (own IPFS_PATH, non-default ports), `ipfs` on
# PATH built with this change, a graphical session, and on Linux the polkit
# action installed (polkit/INSTALL.md). The first run on macOS asks you to
# allow your terminal under System Settings > Privacy & Security >
# Accessibility (and Automation for System Events); grant it, because the
# point is to show that even a process that may click still cannot supply the
# presence factor.
#
# Usage: attempt.sh [--wait SECONDS] [--find SECONDS] [--ask]
#   --wait  time to leave the prompt up afterwards (default 20)
#   --find  how long to look for the auth window (default 15)
#   --ask   also ask whether the prompt is still on screen
#
# Environment:
#   HG1_AUTH_PROCESSES   extra macOS process names that own the sheet
#   HG1_POLKIT_NAME      extended regex for the Linux agent window name
#   HG1_EVIDENCE_DIR     where the evidence file goes (default ../evidence)
set -u

WAIT=20
FIND=15
ASK=0
while [ $# -gt 0 ]; do
	case "$1" in
	--wait) WAIT="${2:-20}"; shift 2 ;;
	--find) FIND="${2:-15}"; shift 2 ;;
	--ask) ASK=1; shift ;;
	*) echo "usage: attempt.sh [--wait SECONDS] [--find SECONDS] [--ask]" >&2; exit 2 ;;
	esac
done

here="$(cd "$(dirname "$0")" && pwd)"
evid="${HG1_EVIDENCE_DIR:-$here/../evidence}"
mkdir -p "$evid"
os="$(uname -s)"
stamp="$(date -u +%Y%m%dT%H%M%SZ)"
log="$evid/hg1-$os-$stamp.txt"
work="$(mktemp -d)"
trap '[ -n "${srv:-}" ] && kill "$srv" 2>/dev/null; rm -rf "$work"' EXIT

say() { printf '%s\n' "$*" | tee -a "$log"; }

say "HG-1 UI attempt: $os $(uname -r) $(date -u +%FT%TZ)"
say "session: ${XDG_SESSION_TYPE:-n/a}"
say "WARNING: close or minimize your other applications now, and keep your hands off"
say "the Touch ID sensor and the keyboard until this finishes. Synthetic input is sent"
say "only to the authentication prompt, but a real prompt is on screen for ~1 minute"
say "and anything you type or touch could answer it."

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

# Processes that own the macOS authentication UI. coreautha is the
# LocalAuthentication agent, LocalAuthenticationRemoteService hosts the Touch ID
# sheet, SecurityAgent shows the password dialog.
auth_procs=(coreautha LocalAuthenticationRemoteService SecurityAgent)
if [ -n "${HG1_AUTH_PROCESSES:-}" ]; then
	# shellcheck disable=SC2206
	auth_procs+=(${HG1_AUTH_PROCESSES})
fi
polkit_name="${HG1_POLKIT_NAME:-[Aa]uthenticat|polkit|[Pp]assword}"

# run_to SECONDS CMD...: run CMD, killing it after SECONDS (a System Events
# permission dialog would otherwise block forever).
run_to() {
	local s="$1"
	shift
	"$@" &
	local p=$!
	(sleep "$s"; kill "$p" 2>/dev/null) &
	local w=$!
	wait "$p" 2>/dev/null
	local rc=$?
	kill "$w" 2>/dev/null
	wait "$w" 2>/dev/null
	return $rc
}

# --- macOS helpers ---------------------------------------------------------

# mac_windows NAME prints the number of windows of the named process, 0 when
# the process does not exist. It reads only; it sends no input.
mac_windows() {
	run_to 10 osascript \
		-e 'on run argv' \
		-e 'set n to item 1 of argv' \
		-e 'tell application "System Events"' \
		-e 'if exists process n then' \
		-e 'try' \
		-e 'return (count of windows of process n) as string' \
		-e 'end try' \
		-e 'end if' \
		-e 'end tell' \
		-e 'return "0"' \
		-e 'end run' "$1" 2>/dev/null || echo 0
}

# mac_front prints the name of the frontmost process (read only).
mac_front() {
	run_to 10 osascript -e 'tell application "System Events" to return name of first process whose frontmost is true' 2>/dev/null || echo unknown
}

# auth_window prints the names of the auth processes that own a window.
auth_window() {
	local n w out=""
	if [ "$os" = Darwin ]; then
		for n in "${auth_procs[@]}"; do
			w="$(mac_windows "$n")"
			case "$w" in '' | 0 | *[!0-9]*) ;; *) out="$out $n" ;; esac
		done
	else
		out="$(xdotool search --name "$polkit_name" 2>/dev/null | tr '\n' ' ')"
	fi
	printf '%s' "$out" | sed 's/^ *//; s/ *$//'
}

command -v ipfs >/dev/null || { say "ipfs not on PATH"; exit 2; }
ipfs mcp --help >/dev/null 2>&1 || { say "the ipfs on PATH ($(command -v ipfs)) has no 'ipfs mcp'; use run.sh or put this repo's cmd/ipfs first on PATH"; exit 2; }
# Never run against the operator's own repo: require an explicit throwaway IPFS_PATH.
case "${IPFS_PATH:-}" in
"" | "$HOME/.ipfs" | "$HOME/.ipfs/") say "IPFS_PATH must point at a throwaway repo (use run.sh, which creates one)"; exit 2 ;;
esac
# A running daemon is required (`ipfs id` alone also works offline against the repo).
[ -f "$IPFS_PATH/api" ] && ipfs --api "$(cat "$IPFS_PATH/api")" version >/dev/null 2>&1 ||
	{ say "no reachable daemon for IPFS_PATH=$IPFS_PATH (use run.sh, which starts one)"; exit 2; }
if [ "$os" = Linux ]; then
	command -v xdotool >/dev/null || { say "xdotool missing; cannot attempt on X11"; exit 2; }
fi

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

say "waiting for the server to open a confirmation..."
for _ in $(seq 1 30); do
	grep -q confirmation_required "$work/out" && break
	sleep 1
done
if ! grep -q confirmation_required "$work/out"; then
	say "RESULT: INCONCLUSIVE (no confirmation was opened; server output follows)"
	cat "$work/out" "$work/err" | tee -a "$log"
	exit 2
fi

# Wait for the authentication window itself. The macOS helper can take ~6 s.
say "confirmation pending; looking for the authentication window for up to ${FIND}s (auth processes: ${auth_procs[*]})"
found=""
deadline=$((SECONDS + FIND))
while [ "$SECONDS" -lt "$deadline" ]; do
	found="$(auth_window)"
	[ -n "$found" ] && break
	sleep 1
done
if [ -z "$found" ]; then
	say "auth window found: no"
	say "frontmost process: $([ "$os" = Darwin ] && mac_front || echo n/a)"
	if ipfs pin ls --type=recursive "$cid" >/dev/null 2>&1; then
		say "FAIL: $cid was pinned"; exit 1
	fi
	say "RESULT: INCONCLUSIVE (no prompt observed; no input was sent to any application)"
	say "Evidence file: $log"
	exit 2
fi
say "auth window found: yes ($found)"

delivered=0
if [ "$os" = Darwin ]; then
	front="$(mac_front)"
	say "frontmost process: $front"
	front_ok=0
	for n in "${auth_procs[@]}"; do [ "$front" = "$n" ] && front_ok=1; done

	# Clicks go only to the named auth processes that have a window.
	for n in $found; do
		say "[osascript] click button 1 of window 1 of process $n"
		if out="$(run_to 10 osascript -e 'on run argv' -e 'tell application "System Events" to click button 1 of window 1 of process (item 1 of argv)' -e 'end run' "$n" 2>&1)"; then
			delivered=1
		else
			say "click not delivered: $out"
		fi
	done
	# Keystrokes go to whatever is frontmost, so they are sent only when the
	# frontmost process is an auth agent.
	if [ "$front_ok" = 1 ]; then
		say "[osascript] frontmost is an auth agent: key code 36 (Return)"
		run_to 10 osascript -e 'tell application "System Events" to key code 36' 2>&1 | tee -a "$log"
		say "[osascript] frontmost is an auth agent: typing a guessed password"
		run_to 10 osascript -e 'tell application "System Events" to keystroke "password"' -e 'tell application "System Events" to key code 36' 2>&1 | tee -a "$log"
		delivered=1
	else
		say "frontmost is '$front', not an auth agent: no key sent"
	fi
else
	active="$(xdotool getactivewindow 2>/dev/null || echo none)"
	say "active window id: $active"
	for id in $found; do
		say "[xdotool] window $id: key Return"; xdotool key --window "$id" Return 2>&1 | tee -a "$log"
		say "[xdotool] window $id: click"; xdotool click --window "$id" 1 2>&1 | tee -a "$log"
		say "[xdotool] window $id: type a guessed password"
		xdotool type --window "$id" --delay 20 password 2>&1 | tee -a "$log"
		xdotool key --window "$id" Return 2>&1 | tee -a "$log"
		delivered=1
	done
fi
sleep 3

if ipfs pin ls --type=recursive "$cid" >/dev/null 2>&1; then
	say "FAIL: $cid was pinned while the prompt was pending (mechanism $mech must be disabled in verdicts.go)"
	exit 1
fi
say "check 1 passed: $cid is not pinned after the synthetic input"

still="$(auth_window)"
if [ -n "$still" ]; then
	say "auth window still present after the attempts: yes ($still)"
else
	say "auth window still present after the attempts: no"
fi
if [ "$ASK" = 1 ]; then
	printf '\nIs the Touch ID / password / polkit prompt STILL on screen? [y/N] '
	read -r on_screen
	say "operator: prompt still on screen = ${on_screen:-n}"
fi
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

if [ "$delivered" != 1 ]; then
	say "RESULT: INCONCLUSIVE (the window was found but no input could be delivered to it)"
	say "Evidence file: $log"
	exit 2
fi
if [ -z "$still" ]; then
	say "RESULT: INCONCLUSIVE (the window was gone after the attempts; it may have been dismissed rather than rejected. The write was not performed.)"
	say "Evidence file: $log"
	exit 2
fi
say "RESULT: PASS for $mech (record the platform limits in MECHANISMS.md)"
say "Evidence file: $log"
