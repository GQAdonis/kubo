#!/usr/bin/env bash
# Render RELEASE_NOTES.tmpl.md to dist/RELEASE_NOTES.md using the current version.
set -euo pipefail
HERE="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
ROOT="$(cd "$HERE/../.." && pwd)"
cd "$ROOT"
NUMBER="$(sed -n 's/^const CurrentVersionNumber = "\(.*\)"/\1/p' version.go)"
COMMIT="$(git rev-parse --short HEAD)"
MINVERSION="$(sed -n 's/^  "minKuboVersion": "\(.*\)",/\1/p' mcp/contract/catalog.json | head -1)"
MINVERSION="${MINVERSION:-0.29.0}"
mkdir -p "$HERE/dist"
sed -e "s/@VERSION@/${NUMBER}-${COMMIT}/g" -e "s/@COMMIT@/${COMMIT}/g" \
    -e "s/@NUMBER@/${NUMBER}/g" -e "s/@MINVERSION@/${MINVERSION}/g" \
    "$HERE/RELEASE_NOTES.tmpl.md" > "$HERE/dist/RELEASE_NOTES.md"
if grep -n '@[A-Z]*@' "$HERE/dist/RELEASE_NOTES.md"; then echo "unrendered placeholder" >&2; exit 1; fi
echo "wrote $HERE/dist/RELEASE_NOTES.md (min daemon $MINVERSION)"
