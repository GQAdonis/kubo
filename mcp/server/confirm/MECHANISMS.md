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
| Windows | none | no | n/a | n/a | no resolver this phase: Desktop's channel client is inert on `win32`; non-eliciting writes get `confirmation_unavailable` |
| Paired Desktop channel | `desktop` | no | automated IPC cases (test/cli `TestMCPConfirmHG1`) | failed for a bare click (2026-10-12, below); Touch ID in the Desktop window added | opt-in (name `desktop`); write-class only, never a presence resolver; channel auth version 2 (below) |
| `ipfs mcp confirm` | `cli` | no | automated IPC cases | n/a (explicit weak opt-in) | enabled with `--confirm=cli` only |

Defaults. With no `--confirm` and no `IPFS_MCP_CONFIRM`, the gate is on:
`native`. macOS uses LocalAuthentication when `/usr/bin/swift` exists,
otherwise it fails closed with a message naming the Xcode command line tools
(`xcode-select --install`). Linux uses polkit only when the action is
installed, otherwise it fails closed. Windows has no resolver this phase.
`desktop` is opt-in since 2026-10-12: name it in `--confirm`, `IPFS_MCP_CONFIRM`
or `confirmation.modes`.
A mechanism the host cannot offer is skipped and a client that cannot elicit
gets `confirmation_unavailable`. `cli` is never a default; name it with
`--confirm=cli`. The off switch is `--confirm=none` or `IPFS_MCP_CONFIRM=none`
(the flag wins over the variable). Automated tests set
`IPFS_MCP_CONFIRM=none` so no test shows a dialog.

`requires_native_prompt` (a destructive call from a `ui` or `a2a` principal)
is satisfied only by a presence-factor resolver: `macos-localauth` or
`linux-polkit` (`server.IsPresenceResolver`). `desktop` never counts (ADR-0003,
amendment of 2026-10-12). `cli` and `agui-interrupt` never satisfy it. With only those
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

No native presence factor is implemented, and IPFS Desktop's channel client
is inert on `win32` (Node cannot reach the Unix-socket channel there). Windows
has no out-of-band resolver this phase: `--confirm=native` yields a resolver
that is always unavailable, and a non-eliciting client gets
`confirmation_unavailable`. Writes work from clients that elicit. A Windows
transport for Desktop is a later change.

## Paired Desktop channel

HTTP over a Unix socket in a private (0700) directory, one socket per server
process (`<pid>-<n>.sock`). The server stores only a verifier,
`V = SHA-256(pairing key)` (32 bytes, hex in the pairing file). `ipfs mcp pair`
prints the key once; Desktop keeps it in `safeStorage` and derives `V` itself.

Decided 2026-10-12 (ADR-0003, amendment of that date):

- `desktop` is **opt-in**. The default modes are `native` only; name `desktop`
  in `--confirm`, `IPFS_MCP_CONFIRM` or `confirmation.modes` to use it.
- `desktop` is **not a presence resolver**, now or when the window ships. It
  resolves write-class confirmations only; a confirmation that requires the
  native prompt is never offered to it.
- Desktop asks for Touch ID (`systemPreferences.promptTouchID`) in its own
  window before it sends an approval, arms Approve only after a delay, does
  not steal focus, and sets content protection on the window. These are
  Desktop-side checks; the server cannot verify them, which is why `desktop`
  stays write-only.
- Windows: Desktop's channel client is inert on `win32`, so Windows has no
  resolver this phase (see Windows above).

### Channel authentication, version 2

Version 2 replaces the bearer key. The key never crosses the socket. Both sides ship
in this fork together, so there is no negotiation and no fallback: a server
accepts only version 2 for `desktop`, and Desktop speaks only version 2.

**Key.** `K = V`, the 32 raw bytes of `SHA-256(UTF-8 pairing key)`. Desktop
computes it from the key; the server reads it from the pairing file. All HMACs
are HMAC-SHA-256 with key `K`. Consequence, accepted: the pairing file (mode
0600 under the operator's account) is now equivalent to the key. A same-user
process could already replace it (accepted residual risk, ADR-0003 amendment
of 2026-10-11); with version 2 it can also read it and act as Desktop without
leaving a trace. Keep the file private. A public-key scheme (Desktop signs, the
server stores only a public key) would remove this; it is a possible version 3.

**Request headers** (Desktop to server, every request):

| Header | Value |
|---|---|
| `X-IPFS-MCP-Auth-Version` | `2` |
| `X-IPFS-MCP-Timestamp` | Unix time in whole seconds, decimal ASCII, no sign, no leading zeros |
| `X-IPFS-MCP-Nonce` | 16 random bytes from a CSPRNG, base64url without padding (22 characters) |
| `X-IPFS-MCP-Signature` | base64url without padding of `HMAC(K, request string)` (43 characters) |

**Request string.** These fields joined with a single `\n` (0x0A), no trailing
newline, all ASCII:

```
ipfs-mcp-confirm-v2
request
<socket>        base name of the socket the client connected to, e.g. 4242-1.sock
<METHOD>        GET or POST, upper case
<path>          exact request path, e.g. /v1/pending (no query string; any query is rejected)
<timestamp>     the X-IPFS-MCP-Timestamp value, byte for byte
<nonce>         the X-IPFS-MCP-Nonce value, byte for byte
<body-sha256>   lower-case hex SHA-256 of the raw request body bytes (of the empty string for GET)
```

The socket name binds a request to one server, so a signed request captured by
a rogue listener cannot be replayed to another socket.

**Server checks, in this order** (body read first, limit 4 KiB as today):

1. `Authorization` present: reject (v1 bearer retired).
2. Version header missing while the cli marker is absent: `auth_required`.
3. Version not exactly `2`: `auth_version_unsupported`.
4. Timestamp, nonce or signature malformed (wrong length, not base64url, not
   decimal): `auth_malformed`.
5. `|server time - timestamp| > 30 s`: `auth_stale`.
6. No valid pairing (missing or invalid file): `auth_invalid`.
7. Signature does not match (constant-time compare): `auth_invalid`.
8. Nonce already in the replay cache: `auth_replayed`.
9. Insert the nonce, then serve.

**Replay cache.** Per server process, in memory. Each accepted nonce is kept
for 60 s (twice the timestamp window), then dropped. At most 4096 entries;
when full after dropping expired entries, new requests get
`auth_busy` (HTTP 503 with `Retry-After: 1`) and nothing is evicted early.
Desktop polls about once a second, so a real client uses about 60 entries. A
restart empties the cache; the 30 s window bounds replay across a restart.

**Server proof** (server to Desktop, on every response to a request that passed
check 4, while a valid pairing exists): header `X-IPFS-MCP-Server-Proof`,
base64url without padding of `HMAC(K, response string)`:

```
ipfs-mcp-confirm-v2
response
<socket>
<nonce>          the request's nonce
<status>         HTTP status code, decimal
<body-sha256>    lower-case hex SHA-256 of the raw response body bytes
```

Desktop verifies the proof (constant time) before it reads or shows anything
from a response. A response without a valid proof means the socket does not
hold the verifier: Desktop ignores that socket for the rest of the session
(until a new pairing), shows nothing from it, never sends it a resolve, and
logs the event once.

**Test vector** (both implementations must reproduce it; computed with Python
`hmac`/`hashlib` on 2026-10-12):

- pairing key `example-pairing-key`; `K` hex
  `bb7a643be3798381e468615dc1397a9e2cb519d7d2a927fcb7f5ea7a7a6d847f`
- socket `4242-1.sock`, `GET /v1/pending`, timestamp `1760270400`, nonce bytes
  0x00..0x0f, so nonce `AAECAwQFBgcICQoLDA0ODw`, empty body
- signature `kJV7qp4TLPIC1phDIUnyT_W0YR9_eMnJlKfzv4uWtzs`
- response status 200, body `{"pending":[]}` followed by one `\n` (as Go's
  `json.Encoder` writes it): server proof
  `paaByo8rrHXnI8pwzkw0SFyWQ7DUnkM3CwbGy0bZsbs`

**Errors.** Every rejection is JSON `{"error": "<message>", "code": "<code>"}`
(`code` is new and additive):

| Code | HTTP | Meaning |
|---|---|---|
| `auth_required` | 401 | No authentication headers and no cli marker |
| `auth_version_unsupported` | 401 | `Authorization: Bearer` (v1) or a version other than 2; response header `X-IPFS-MCP-Auth-Versions: 2` |
| `auth_malformed` | 400 | A header has the wrong form, or the path has a query |
| `auth_stale` | 401 | Timestamp outside +/- 30 s |
| `auth_invalid` | 403 | Not paired, or the signature does not match |
| `auth_replayed` | 401 | Nonce seen within 60 s |
| `auth_busy` | 503 | Replay cache full |

**cli path, unchanged.** `ipfs mcp confirm` sends `X-IPFS-MCP-Confirm: cli` and
no key (`cli.go`); it is accepted only when cli confirmation is on. A request
that carries version-2 headers is authenticated as `desktop` only; the cli
marker on it is ignored.

### Pending view

`GET /v1/pending` returns `{"pending": [PendingView...]}`. `PendingView` gains
`targets` (additive, R14 security M-2): the confirmation summary's structured
targets, always present, possibly empty:

```json
"targets": [{"kind": "cid", "value": "bafy..."}, {"kind": "mfs_path", "value": "/docs/a.txt"}]
```

`kind` is one of the contract's `Target` kinds (`confirmation.schema.json`
`$defs/Target`: `cid`, `content_path`, `mfs_path`, `key`, `ipns_name`, `peer`,
`repo`, `index`, `content`). Desktop renders targets from this field with their
kind labels and stops extracting CIDs from the summary text.

### Other server rules

- The socket directory is created and checked with `Lstat`; a symlink is
  refused (R14 security L-4).
- `ipfs mcp pair` needs a terminal on stdin and refuses while any server
  accepts connections on a socket in the confirmation directory; a stale socket
  file does not block it. A new pairing replaces the old one, and the channel
  re-reads the verifier on every request, so the old key stops working on
  running servers at once.
- Automated cases: `TestMCPConfirmHG1` (no key, forged key, `ipfs mcp confirm`
  without cli confirmation). Version 2 adds cases for a bearer request, a stale
  timestamp, a replayed nonce, a signature for another socket, a tampered
  body, and a server proof checked by a test client.

### HG-1 evidence (2026-10-12)

During the Desktop end-to-end run on a throwaway test node, an unknown
same-user process (computer-use agents run on this Mac) clicked Desktop's
Approve 4 to 6 seconds after the window appeared. A click in a window is not
a presence factor. This is the evidence behind the decisions above.

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
