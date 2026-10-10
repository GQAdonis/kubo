#!/usr/bin/env bash
# Local release build for the fork's `ipfs` binary (MCP use).
# Run from the repository root:  mcp/release/build.sh
# Builds through `make build` (never plain `go build`) so ldflags are kept.
# Publishing, tagging and uploading are operator actions and are not done here.
#
# Environment:
#   TARGETS   space separated goos/goarch list (default: the five release targets)
#   GOTAGS    extra build tags passed to make (default: none)
set -euo pipefail

ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)"
cd "$ROOT"
[ -f Makefile ] && [ -d cmd/ipfs ] || { echo "run from the kubo repo root" >&2; exit 1; }

DIST="$ROOT/mcp/release/dist"
STAGE="$DIST/.stage"
HOST_BIN="$ROOT/cmd/ipfs/ipfs"
TARGETS="${TARGETS:-darwin/arm64 darwin/amd64 linux/amd64 linux/arm64 windows/amd64}"

NUMBER="$(sed -n 's/^const CurrentVersionNumber = "\(.*\)"/\1/p' version.go)"
[ -n "$NUMBER" ] || { echo "cannot read CurrentVersionNumber from version.go" >&2; exit 1; }
COMMIT="$(git rev-parse --short HEAD)"
VERSION="${NUMBER}-${COMMIT}"

restore_host() {
  # Leave cmd/ipfs/ipfs as a host build whatever happened above.
  rm -f "$HOST_BIN"
  env -u GOOS -u GOARCH make build >/dev/null
}
trap 'rc=$?; restore_host || echo "WARNING: host rebuild failed" >&2; rm -rf "$STAGE"; exit $rc' EXIT

rm -rf "$DIST"
mkdir -p "$STAGE"

write_readme() { # dir goos
  cat > "$1/README.md" <<RD
# kubo-mcp ${VERSION}

This archive holds the fork's \`ipfs\` binary. Use it as an MCP client of an
existing Kubo node: point your agent harness at \`ipfs mcp\` and it talks to a
running daemon over that daemon's RPC API.

    ipfs mcp --api /ip4/127.0.0.1/tcp/5001

It needs a running Kubo daemon 0.29.0 or newer. Docs: docs/mcp/ in the source
tree (ADR-0001 and ADR-0003 explain placement, confirmation and auth).
Verify the download with SHA256SUMS before running it. Licenses: LICENSE,
LICENSE-APACHE, LICENSE-MIT.
RD
}

for t in $TARGETS; do
  goos="${t%/*}"; goarch="${t#*/}"
  name="kubo-mcp_${VERSION}_${goos}-${goarch}"
  echo "==> $t"
  rm -f "$HOST_BIN"
  GOOS="$goos" GOARCH="$goarch" CGO_ENABLED=0 make build >/dev/null
  [ -f "$HOST_BIN" ] || { echo "make build produced no binary for $t" >&2; exit 1; }
  pkg="$STAGE/$name"
  mkdir -p "$pkg"
  exe=ipfs; [ "$goos" = windows ] && exe=ipfs.exe
  mv "$HOST_BIN" "$pkg/$exe"
  cp LICENSE LICENSE-APACHE LICENSE-MIT "$pkg/"
  write_readme "$pkg"
  if [ "$goos" = windows ]; then
    (cd "$STAGE" && zip -qr "$DIST/$name.zip" "$name")
  else
    tar -C "$STAGE" -czf "$DIST/$name.tar.gz" "$name"
  fi
done

(cd "$DIST" && shasum -a 256 kubo-mcp_* > SHA256SUMS)
echo "version: $VERSION"
ls -l "$DIST"
