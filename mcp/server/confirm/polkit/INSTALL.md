# Installing the polkit action (Linux)

`ipfs mcp --confirm=native` on Linux asks polkit to authenticate the person at
the machine before an agent's write or destructive call runs. Kubo does not
install the action: it is a one-time root step that you do on purpose. Without
it the server refuses such calls with `confirmation_unavailable`.

## Install

```sh
sudo install -m 0644 org.ipfs.kubo.mcp.confirm.policy \
  /usr/share/polkit-1/actions/org.ipfs.kubo.mcp.confirm.policy
pkaction --action-id org.ipfs.kubo.mcp.confirm   # prints the action
```

## What it needs

- A graphical or console session with a polkit authentication agent (for
  example `polkit-gnome-authentication-agent-1`, the KDE agent or the one in
  your desktop environment). Over ssh, or with no agent, `pkcheck` cannot ask
  anyone and the server refuses.
- `pkaction` and `pkcheck` from the `polkit` package.

## Behavior

- The action is `auth_self` with no caching (`allow_any` and `allow_inactive`
  are `no`), so each confirmation prompts for the user's own password.
- The prompt text names the exact operation and the CIDs it touches.
- A process under the same user cannot answer the prompt: the agent that
  shows it runs outside the calling process and checks the password.

## Remove

```sh
sudo rm /usr/share/polkit-1/actions/org.ipfs.kubo.mcp.confirm.policy
```

## Wayland note

The prompt is rendered by your authentication agent, not by Kubo. See
`../MECHANISMS.md` for the recorded limits of synthetic input on X11 and
Wayland.
