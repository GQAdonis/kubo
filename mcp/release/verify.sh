#!/usr/bin/env bash
# Verify mcp/release/dist/SHA256SUMS against the archives next to it.
set -euo pipefail
DIST="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)/dist"
[ -f "$DIST/SHA256SUMS" ] || { echo "no SHA256SUMS in $DIST; run build.sh first" >&2; exit 1; }
cd "$DIST"
shasum -a 256 -c SHA256SUMS
