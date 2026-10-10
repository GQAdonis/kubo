#!/usr/bin/env bash
# Copy the IPFS skills from mcp/skills (source of truth) into the plugin root.
# Plugin hosts load skills from inside the plugin directory, and a path that
# escapes the plugin root does not load, so the plugin carries real copies.
#   mcp/plugin/sync-skills.sh          refresh the copies
#   mcp/plugin/sync-skills.sh --check  exit 1 if the copies differ from source
set -euo pipefail
here="$(cd "$(dirname "$0")" && pwd)"
src="$here/../skills"
dst="$here/ipfs/skills"
mode="${1:-sync}"

skills=()
for d in "$src"/*/; do
  if [ -f "$d/SKILL.md" ]; then skills+=("$(basename "$d")"); fi
done
[ "${#skills[@]}" -gt 0 ] || { echo "no skills found under $src" >&2; exit 2; }

if [ "$mode" = "--check" ]; then
  rc=0
  for s in "${skills[@]}"; do
    diff -r "$src/$s" "$dst/$s" >/dev/null 2>&1 || { echo "out of sync: $s" >&2; rc=1; }
  done
  for d in "$dst"/*/; do
    [ -d "$d" ] || continue
    [ -d "$src/$(basename "$d")" ] || { echo "stale copy: $(basename "$d")" >&2; rc=1; }
  done
  if [ "$rc" -eq 0 ]; then echo "skills in sync (${#skills[@]})"; fi
  exit "$rc"
fi

rm -rf "$dst"
mkdir -p "$dst"
for s in "${skills[@]}"; do
  cp -R "$src/$s" "$dst/$s"
done
echo "synced ${#skills[@]} skills into $dst"
