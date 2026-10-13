#!/usr/bin/env bash
# Verify a release directory: first the SSH signature on SHA256SUMS, then the
# archive checksums.
#
# Usage: verify.sh [--allow-unsigned] [--allowed-signers FILE] [DIST_DIR]
#   DIST_DIR           directory holding SHA256SUMS, SHA256SUMS.sig and the
#                      archives (default: mcp/release/dist next to this script)
#   --allowed-signers  allowed_signers file to trust (default: the one beside
#                      this script). Use a key obtained independently of the
#                      download; trusting a file from the same download as the
#                      archives proves nothing.
#   --allow-unsigned   continue with only the checksum step when
#                      SHA256SUMS.sig is missing (prints a warning)
# Environment: KUBO_MCP_SIGNER overrides the signer identity (default: the
# first key line's identity in the allowed_signers file).
set -euo pipefail

HERE="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
# shellcheck source=signing.sh
. "$HERE/signing.sh"

DIST="$HERE/dist"
ALLOWED="$HERE/allowed_signers"
ALLOW_UNSIGNED=0

while [ $# -gt 0 ]; do
  case "$1" in
    --allow-unsigned) ALLOW_UNSIGNED=1 ;;
    --allowed-signers)
      [ $# -ge 2 ] || { echo "--allowed-signers needs a file" >&2; exit 2; }
      ALLOWED="$2"; shift ;;
    -h|--help) sed -n '2,15p' "${BASH_SOURCE[0]}"; exit 0 ;;
    -*) echo "unknown option: $1" >&2; exit 2 ;;
    *) DIST="$1" ;;
  esac
  shift
done

DIST="$(cd "$DIST" 2>/dev/null && pwd)" || { echo "no such directory: $DIST" >&2; exit 1; }
[ -f "$DIST/SHA256SUMS" ] || { echo "no SHA256SUMS in $DIST; run build.sh first" >&2; exit 1; }

if [ -f "$DIST/SHA256SUMS.sig" ]; then
  [ -f "$ALLOWED" ] || { echo "allowed_signers file not found: $ALLOWED" >&2; exit 1; }
  # First non-comment, non-blank line's principal.
  FIRST_PRINCIPAL="$(awk '!/^[[:space:]]*(#|$)/ {print $1; exit}' "$ALLOWED")"
  [ -n "$FIRST_PRINCIPAL" ] || {
    echo "no key line in $ALLOWED (only comments); add the release public key or pass --allowed-signers FILE" >&2
    exit 1
  }
  IDENTITY="${KUBO_MCP_SIGNER:-$FIRST_PRINCIPAL}"
  command -v ssh-keygen >/dev/null 2>&1 || { echo "ssh-keygen not found (OpenSSH 8.1+ required)" >&2; exit 1; }
  if ! ssh-keygen -Y verify -f "$ALLOWED" -I "$IDENTITY" -n "$SIGN_NAMESPACE" \
        -s "$DIST/SHA256SUMS.sig" < "$DIST/SHA256SUMS"; then
    echo "SIGNATURE VERIFICATION FAILED for $DIST/SHA256SUMS (identity $IDENTITY)" >&2
    exit 1
  fi
elif [ "$ALLOW_UNSIGNED" = 1 ]; then
  echo "WARNING: no SHA256SUMS.sig; verifying checksums only. This proves nothing about who built the archives." >&2
else
  echo "no SHA256SUMS.sig in $DIST; refusing to verify unsigned release (use --allow-unsigned to check checksums only)" >&2
  exit 1
fi

cd "$DIST"
shasum -a 256 -c SHA256SUMS
