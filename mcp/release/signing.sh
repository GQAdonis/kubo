#!/usr/bin/env bash
# Shared signing settings and helpers. Sourced by build.sh and verify.sh.
# SSH signing of SHA256SUMS (OpenSSH 8.1+ `ssh-keygen -Y`).

# The signature namespace binds a signature to this use, so it cannot be
# replayed for another purpose. Change it only here.
SIGN_NAMESPACE="kubo-mcp-release"

# signing_preflight: fail early (before any build) when no signing key is
# configured, unless the operator explicitly opts out with KUBO_MCP_UNSIGNED=1.
# Sets SIGN_ENABLED to 1 or 0.
signing_preflight() {
  SIGN_ENABLED=1
  if [ -z "${KUBO_MCP_SIGNING_KEY:-}" ]; then
    if [ "${KUBO_MCP_UNSIGNED:-}" = 1 ]; then
      SIGN_ENABLED=0
      echo "WARNING: KUBO_MCP_UNSIGNED=1; this release will be UNSIGNED (no SHA256SUMS.sig)." >&2
      return 0
    fi
    echo "KUBO_MCP_SIGNING_KEY is not set." >&2
    echo "Set it to the path of the SSH private key that signs releases, or set KUBO_MCP_UNSIGNED=1 to build an unsigned release." >&2
    return 1
  fi
  if [ ! -f "$KUBO_MCP_SIGNING_KEY" ]; then
    echo "KUBO_MCP_SIGNING_KEY does not point to a file: $KUBO_MCP_SIGNING_KEY" >&2
    return 1
  fi
  command -v ssh-keygen >/dev/null 2>&1 || { echo "ssh-keygen not found (OpenSSH 8.1+ required)" >&2; return 1; }
}

# sign_sums <dist-dir>: writes SHA256SUMS.sig next to SHA256SUMS. ssh-keygen may
# prompt for a passphrase or use ssh-agent; no passphrase is passed here.
sign_sums() {
  local dir="$1"
  if [ "${SIGN_ENABLED:-1}" != 1 ]; then
    echo "WARNING: release is unsigned (KUBO_MCP_UNSIGNED=1)." >&2
    return 0
  fi
  rm -f "$dir/SHA256SUMS.sig"
  (cd "$dir" && ssh-keygen -Y sign -f "$KUBO_MCP_SIGNING_KEY" -n "$SIGN_NAMESPACE" SHA256SUMS)
  [ -f "$dir/SHA256SUMS.sig" ] || { echo "signing produced no SHA256SUMS.sig" >&2; return 1; }
}
