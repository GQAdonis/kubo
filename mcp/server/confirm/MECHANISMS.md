# Native confirmation mechanisms

Task 1.1 of `native-confirmation-prompt`. This file records, per mechanism,
what was verified, what a same-user background process can and cannot do to it
(HG-1), and the state of the code switch. A mechanism whose HG-1 check fails is
disabled in `verdicts.go` (`hg1Disabled`); its resolver then refuses every
request.

Rule for every row: the server approves only when the mechanism itself says
the person is present. Synthetic input, IPC messages, tokens and a repeated
call never approve. If a mechanism is missing, the call is refused with
`confirmation_unavailable`.

## Status

| Mechanism | Resolver | cgo | Probe on a real machine | Real-prompt HG-1 | Code state |
|---|---|---|---|---|---|
| macOS LocalAuthentication | `macos-localauth` | no | done (below) | pending, operator runs `hg1/attempt.sh` | on by default (`native`) |
| Linux polkit | `linux-polkit` | no | not run: no Linux machine in this environment | pending, operator | on by default (`native`) once the action is installed; refuses without it |
| Windows | none | no | n/a | n/a | refuses unless paired with IPFS Desktop (pairing is a later change) |
| Paired Desktop channel | `desktop` | no | automated IPC cases (test/cli `TestMCPConfirmHG1`) | needs Desktop (later) | on by default (`desktop`) once paired; resolves write-class confirmations only, not a presence resolver (see below) |
| `ipfs mcp confirm` | `cli` | no | automated IPC cases | n/a (explicit weak opt-in) | enabled with `--confirm=cli` only |

Defaults. With no `--confirm` and no `IPFS_MCP_CONFIRM`, the gate is on:
`native,desktop`. macOS uses LocalAuthentication when `/usr/bin/swift` exists,
otherwise it fails closed with a message naming the Xcode command line tools
(`xcode-select --install`). Linux uses polkit only when the action is
installed, otherwise it fails closed. Windows uses `desktop` only when paired.
A mechanism the host cannot offer is skipped and a client that cannot elicit
gets `confirmation_unavailable`. `cli` is never a default; name it with
`--confirm=cli`. The off switch is `--confirm=none` or `IPFS_MCP_CONFIRM=none`
(the flag wins over the variable). Automated tests set
`IPFS_MCP_CONFIRM=none` so no test shows a dialog.

`requires_native_prompt` (a destructive call from a `ui` or `a2a` principal)
is satisfied only by a presence-factor resolver: `macos-localauth` or
`linux-polkit` (`server.IsPresenceResolver`). `desktop` does not count until a
Desktop confirmation window consumes the pairing key (ADR-0003, R11
amendment). `cli` and `agui-interrupt` never satisfy it. With only those
available, the call is `confirmation_unavailable`. The broker enforces this on
`Resolve`, and the gate enforces it when choosing resolvers.

Elicitation is used only for `human-mcp` principals, which include stdio. A
`ui`, `a2a` or `uar` principal goes to these out-of-band resolvers even if it
declares elicitation.

At most 2 out-of-band confirmations may be pending per principal, and 2 per
session. A call beyond that gets `confirmation_pending` with reason
`too_many_open_confirmations`, and no prompt is shown.

Every helper process (`/usr/bin/swift`, `pkcheck`, `pkaction`) is started with
an environment built from an allowlist (`exec_env.go`):

- `PATH` is fixed to `/usr/bin:/bin:/usr/sbin:/sbin`.
- Only `HOME`, `TMPDIR`, `USER`, `LOGNAME`, `LANG`, `LC_ALL`, `LC_CTYPE` and
  `__CF_USER_TEXT_ENCODING` are kept.
- `DEVELOPER_DIR`, `TOOLCHAINS`, `SDKROOT`, `DYLD_*` and `LD_*` are always
  dropped.

An MCP config's `env` block therefore cannot redirect a helper.

## macOS: LocalAuthentication without cgo

Mechanism chosen: the server runs the system interpreter
`/usr/bin/swift -` and writes an embedded script (`la_prompt.swift`) to its
stdin. The script calls `LAContext.evaluatePolicy(.deviceOwnerAuthentication)`:
Touch ID, an Apple Watch or the login password, shown by the system (coreauthd),
not by Kubo. Exit code 0 means authenticated, 3 not authenticated, 2
unavailable. Choices and why:

- No cgo, and no helper binary on disk. The script lives in the Go binary and
  is piped on every use, so a same-user process has no file to replace.
  (A same-user process can still replace the `ipfs` binary, which is outside
  this threat model: it already owns the account. `/usr/bin/swift` is run by
  absolute path, so `PATH` does not choose it.)
- Needs the Xcode command line tools (`xcode-select --install`) for
  `/usr/bin/swift`. Without them the probe fails and the server refuses.
- The interpreter runs with the allowlisted environment above. `DEVELOPER_DIR`,
  `TOOLCHAINS` and `SDKROOT` would otherwise let `xcrun` pick a different
  toolchain.
- Cost: the interpreter takes about 6 s to start (measured). A first probe
  runs once per server and is bounded at 30 s, independent of the request
  that triggered it; a probe that does not finish makes the resolver
  unavailable. Each prompt starts the interpreter again. This is the
  price of avoiding cgo and a bundled helper. An opt-in cgo or signed helper
  can replace it later behind the same `factor` interface.
- JXA (`osascript -l JavaScript`) was rejected: it cannot pass the ObjC block
  that `evaluatePolicy` needs.
- `osascript ... with administrator privileges` was rejected: it grants root to
  a throwaway command and is not LocalAuthentication.

Probe recorded 2026-10-09 on macOS 26.7.1 (Darwin 25.6.0), this development
machine, from the same script path (probe mode, no dialog shown):

```
canEvaluate(deviceOwnerAuthentication): true
canEvaluate(deviceOwnerAuthenticationWithBiometrics): true
```

HG-1 expectation (to be confirmed by `hg1/attempt.sh`, not yet run): a process
granted Accessibility can post a click or Return to the dialog and can press
Cancel, but it cannot produce a Touch ID match, and it does not know the
password; a guessed password fails. LocalAuthentication reports not
authenticated and the server denies. Platform limits to record with the
evidence: an Accessibility-permitted process can click; it cannot supply the
presence factor; a session with no GUI (ssh, no console user) cannot show the
prompt and fails closed.

## Linux: polkit

`pkcheck --action-id org.ipfs.kubo.mcp.confirm --process <server pid>
--allow-user-interaction --detail summary <text>`. The action is installed by
the operator (`polkit/org.ipfs.kubo.mcp.confirm.policy`, `polkit/INSTALL.md`):
`auth_self`, no caching, `allow_any` and `allow_inactive` are `no`.

The tools run only from `/usr/bin/pkaction` and `/usr/bin/pkcheck`, never from
a `$PATH` lookup. Each file and its directory, after resolving symlinks, must
be owned by root and not writable by group or others. This is checked when the
resolver is probed and again before every `pkcheck` call. The tools run with
the allowlisted environment above.

The server checks `pkaction --action-id ...` first. Any of the following is
`confirmation_unavailable`, never an approval:

- a missing action;
- a missing or untrusted helper;
- no authentication agent in the session (`pkcheck` exit 3);
- any execution error.

HG-1 expectation (pending operator run on a Linux desktop): the dialog is shown
by the session's authentication agent, which checks the user's password through
polkit's setuid helper. A same-user background process can run `pkcheck`
itself, but that only authorizes its own process, never the server's pending
confirmation. On X11, `xdotool` can press Cancel or type into the dialog but
does not know the password. On Wayland, synthetic input from a background
process is unavailable; this is recorded as `unavailable`, not as a pass.

## Windows

No native presence factor is implemented. `--confirm=native` yields a resolver
that is always unavailable; confirmation works only through the paired IPFS
Desktop window, which lands in a later change. Until then a non-eliciting
client gets `confirmation_unavailable`.

## Paired Desktop channel

HTTP over a Unix socket in a private (0700) directory, one socket per server
process. A client authenticates with `Authorization: Bearer <pairing key>`. The
server stores only a SHA-256 of the key (`ipfs mcp pair` prints the key once;
Desktop keeps it in `safeStorage`). An unpaired client can neither list nor
resolve; a wrong key gets 403; the cli marker is 403 unless `--confirm=cli`.
A same-user process can reach the socket but cannot approve without the key.
Automated cases: `TestMCPConfirmHG1` (no key, forged key, `ipfs mcp confirm`
without `--confirm=cli`).

No Desktop confirmation window consumes the key yet, so the key alone is the
whole factor. For that reason `desktop` is not a presence resolver. It resolves
write-class confirmations but never a destructive call that requires the
native prompt. It becomes a presence resolver only when the Desktop window
ships and adds a user gesture of its own.

`ipfs mcp pair` needs a terminal on stdin. It refuses while any server accepts
connections on a socket in the confirmation directory; a stale socket file
does not block it. A new pairing replaces the old one. The channel re-reads
the stored verifier on every request, so the old key stops working on running
servers at once.

## `ipfs mcp confirm` (`--confirm=cli`)

The explicit headless opt-in. The command refuses without `--confirm=cli` and
without a terminal on stdin, and reads the answer from that terminal only. It
carries no secret, so its factor is the terminal itself: a same-user process
that allocates a pseudo-terminal can drive it. That is the stated cost of the
opt-in; use native or Desktop wherever there is a screen. The server rejects
the cli marker unless it was started with `--confirm=cli`.

## Recording a failure

If `hg1/attempt.sh` fails for a mechanism, add it to `hg1Disabled` in
`verdicts.go` with the reason, save the evidence under `evidence/`, and update
the row above. The resolver refuses from then on.
