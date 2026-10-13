#!/usr/bin/env python3
"""Check the mcp/skills tree against mcp/contract/catalog.json.

Run from anywhere:  python3 mcp/skills/scripts/check_skills.py

Checks, per skill directory (a directory holding SKILL.md):
  - frontmatter has name (equal to the directory name, [a-z0-9-], <= 64),
    description (<= 1024), license, compatibility, metadata;
  - the body is under 500 lines;
  - every file referenced as references/<file> exists, and references
    are one level deep (no nested directories under references/);
  - every tool-like token (ipfs_*, pin_*, ipns_*, key_*, mfs_*, dag_*, car_*,
    routing_*, swarm_*, stats_*, repo_*) is a catalog tool, an input or output
    property name, or an error code or reason from the contract;
  - metadata.tools lists only catalog tools;
  - no hardcoded http(s) URL appears (endpoints are never baked in).
Also checks that every catalog group with tools is covered by some skill.
Exit status is non-zero when any check fails.
"""
import json
import re
import sys
from pathlib import Path

SKILLS = Path(__file__).resolve().parent.parent
CONTRACT = SKILLS.parent / "contract"
PREFIX = re.compile(r"\b(?:ipfs|pin|ipns|key|mfs|dag|car|routing|swarm|stats|repo)_[a-z0-9_]+\b")
NAME = re.compile(r"^[a-z0-9]+(?:-[a-z0-9]+)*$")
URL = re.compile(r"https?://")


def frontmatter(text):
    m = re.match(r"^---\n(.*?)\n---\n(.*)$", text, re.S)
    if not m:
        return None, text
    fm, cur_key, nested = {}, None, None
    for line in m.group(1).split("\n"):
        if re.match(r"^\S", line):
            k, _, v = line.partition(":")
            cur_key = k.strip()
            fm[cur_key] = v.strip() if v.strip() else {}
            nested = not v.strip()
        elif nested and cur_key:
            k, _, v = line.strip().partition(":")
            fm[cur_key][k.strip()] = v.strip()
    return fm, m.group(2)


def contract_vocabulary():
    catalog = json.loads((CONTRACT / "catalog.json").read_text())
    tools = {t["name"] for t in catalog["tools"]}
    words = set(tools)
    for path in (CONTRACT / "schemas" / "tools").glob("*.json"):
        data = json.loads(path.read_text())
        words.update(data.get("properties", {}).keys())

        def walk(node):
            if isinstance(node, dict):
                words.update(node.get("properties", {}).keys())
                for v in node.values():
                    walk(v)
            elif isinstance(node, list):
                for v in node:
                    walk(v)

        walk(data)
    common = json.loads((CONTRACT / "schemas" / "common.json").read_text())
    words.update(common["$defs"]["ErrorCode"]["enum"])
    status = json.loads((CONTRACT / "schemas" / "tools" / "ipfs_node_status.output.json").read_text())
    reasons = status["properties"]["unavailable_tools"]["items"]["properties"]["reason"]["enum"]
    words.update(reasons)
    return catalog, tools, words


def main():
    catalog, tools, words = contract_vocabulary()
    errors, used = [], set()
    skill_dirs = sorted(p.parent for p in SKILLS.glob("*/SKILL.md"))
    if not skill_dirs:
        errors.append("no skills found")
    for d in skill_dirs:
        text = (d / "SKILL.md").read_text()
        fm, body = frontmatter(text)
        where = d.name
        if fm is None:
            errors.append(f"{where}: no frontmatter")
            continue
        name = fm.get("name", "")
        if name != d.name or not NAME.match(name) or len(name) > 64:
            errors.append(f"{where}: bad name {name!r}")
        desc = fm.get("description", "")
        if not desc or len(desc) > 1024:
            errors.append(f"{where}: description missing or over 1024 chars ({len(desc)})")
        for key in ("license", "compatibility", "metadata"):
            if not fm.get(key):
                errors.append(f"{where}: missing {key}")
        if len(text.split("\n")) >= 500:
            errors.append(f"{where}: SKILL.md has {len(text.splitlines())} lines (limit 500)")
        for ref in set(re.findall(r"references/[A-Za-z0-9._-]+", text)):
            if not (d / ref).is_file():
                errors.append(f"{where}: missing referenced file {ref}")
        ref_dir = d / "references"
        if ref_dir.exists() and any(p.is_dir() for p in ref_dir.iterdir()):
            errors.append(f"{where}: references/ must be one level deep")
        scan = [(d / "SKILL.md", text)]
        if ref_dir.exists():
            scan += [(p, p.read_text()) for p in sorted(ref_dir.glob("*.md"))]
        for path, content in scan:
            if URL.search(content):
                errors.append(f"{path.relative_to(SKILLS)}: contains a hardcoded http(s) URL")
            for tok in sorted(set(PREFIX.findall(content))):
                if tok in tools:
                    used.add(tok)
                elif tok not in words:
                    errors.append(f"{path.relative_to(SKILLS)}: unknown tool-like name {tok}")
        listed = fm.get("metadata", {}).get("tools", "").split()
        for t in listed:
            if t not in tools:
                errors.append(f"{where}: metadata.tools names unknown tool {t}")
    groups = {t["group"] for t in catalog["tools"]}
    covered = {t["group"] for t in catalog["tools"] if t["name"] in used}
    for g in sorted(groups - covered):
        errors.append(f"catalog group {g} has no tool mentioned by any skill")
    uncovered = sorted(tools - used)
    print(f"skills: {len(skill_dirs)}; catalog tools: {len(tools)}; tools mentioned: {len(used)}")
    if uncovered:
        print("catalog tools not mentioned by any skill:", ", ".join(uncovered))
    for e in errors:
        print("FAIL", e)
    print("OK" if not errors else f"{len(errors)} failure(s)")
    return 1 if errors else 0


if __name__ == "__main__":
    sys.exit(main())
