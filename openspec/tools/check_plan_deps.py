"""Change-level dependencies and rounds (plan revision 2, after review round 2), table augmentation and consistency check.

Usage: deps.py <repo> <table.md in/out> [--mirror]
"""
import os
import re
import sys

ROOT, TABLE = sys.argv[1], sys.argv[2]
MIRROR = "--mirror" in sys.argv
CH = os.path.join(ROOT, "openspec", "changes")

TOOLS = ["mcp-tools-content", "mcp-tools-mfs", "mcp-tools-ipns-keys", "mcp-tools-dag-car", "mcp-tools-routing-diag"]
DEPS = {
    "team-ownership-grant": [],
    "mcp-tool-contract": ["team-ownership-grant"],
    "mcp-architecture-adrs": ["team-ownership-grant"],
    "webmcp-host-spike": ["team-ownership-grant"],
    "uar-conformance-request": ["team-ownership-grant"],
    "mcp-server-core-stdio": ["mcp-tool-contract"],
    "mcp-ui-component-library": ["mcp-tool-contract"],
    "mcp-pinning-with-confirmation": ["mcp-server-core-stdio"],
    "agent-chat-island": ["mcp-ui-component-library"],
    "mcp-http-transport-auth": ["mcp-pinning-with-confirmation"],
    "native-confirmation-prompt": ["mcp-pinning-with-confirmation"],
    **{t: ["mcp-pinning-with-confirmation"] for t in TOOLS},
    "webmcp-tool-package": ["mcp-tool-contract", "mcp-pinning-with-confirmation", "webmcp-host-spike"],
    "mcp-agent-principal-enforcement": ["mcp-http-transport-auth", "mcp-pinning-with-confirmation"],
    "mcp-app-views": ["mcp-ui-component-library", "mcp-tools-content", "mcp-pinning-with-confirmation"],
    "ipfs-agent-skills": TOOLS + ["mcp-pinning-with-confirmation"],
    "webui-webmcp-integration": ["webmcp-tool-package", "webmcp-host-spike"],
    "desktop-agent-chat": ["agent-chat-island", "native-confirmation-prompt"],
    "mcp-llm-proxy-loop": ["mcp-agent-principal-enforcement", "mcp-pinning-with-confirmation"],
    "uar-integration-spike": ["mcp-agent-principal-enforcement", "native-confirmation-prompt", "uar-conformance-request"],
    "webui-cid-delivery": ["webui-webmcp-integration"],
    "mcp-agui-endpoint": ["mcp-llm-proxy-loop", "native-confirmation-prompt"],
    "mcp-a2a-endpoint": ["mcp-llm-proxy-loop", "native-confirmation-prompt"],
    "mcp-embeddings-surrealdb": ["mcp-agent-principal-enforcement", "mcp-llm-proxy-loop", "mcp-tool-contract"],
    "agentic-uar-integration": ["uar-integration-spike"],
    "webui-agent-chat": ["agent-chat-island", "mcp-agui-endpoint", "native-confirmation-prompt", "webui-webmcp-integration"],
    "mcp-embedding-index-lifecycle": ["mcp-embeddings-surrealdb", "mcp-tools-content", "mcp-tools-routing-diag", "mcp-pinning-with-confirmation"],
}
# Every change that alters the `ipfs` binary (Go code under mcp/server or cmd/ipfs/kubo, or webui.go for D2).
BINARY = ["mcp-server-core-stdio", "mcp-pinning-with-confirmation", "mcp-http-transport-auth", "native-confirmation-prompt",
          "mcp-agent-principal-enforcement", "mcp-llm-proxy-loop", "mcp-agui-endpoint", "mcp-a2a-endpoint",
          "mcp-embeddings-surrealdb", "mcp-embedding-index-lifecycle", "mcp-app-views", "webui-cid-delivery"] + TOOLS
DEPS["mcp-release-binaries"] = BINARY
DEPS["mcp-user-docs"] = sorted(set(BINARY + ["mcp-release-binaries", "agent-chat-island", "webui-agent-chat", "desktop-agent-chat"]))
DEPS["mcp-plugin-marketplaces"] = ["ipfs-agent-skills", "mcp-release-binaries", "mcp-http-transport-auth"]
# Added 2026-10-11 (R11 follow-up, operator decision 5): CI runs the whole mcp tree, so it follows every
# binary-changing change, the UI library and chat island it tests, and the skills it checks.
DEPS["mcp-ci-workflow"] = sorted(set(BINARY + ["mcp-ui-component-library", "agent-chat-island", "ipfs-agent-skills"]))
ALL = list(DEPS.keys())
DEPS["phase-boundary-review"] = [c for c in ALL]

ROUND = {
    "team-ownership-grant": 0,
    "mcp-tool-contract": 1, "mcp-architecture-adrs": 1, "webmcp-host-spike": 1, "uar-conformance-request": 1,
    "mcp-server-core-stdio": 2, "mcp-ui-component-library": 2,
    "mcp-pinning-with-confirmation": 3, "agent-chat-island": 3,
    "mcp-http-transport-auth": 4, "native-confirmation-prompt": 4, **{t: 4 for t in TOOLS}, "webmcp-tool-package": 4,
    "mcp-agent-principal-enforcement": 5, "mcp-app-views": 5, "ipfs-agent-skills": 5, "webui-webmcp-integration": 5, "desktop-agent-chat": 5,
    "mcp-llm-proxy-loop": 6, "uar-integration-spike": 6, "webui-cid-delivery": 6,
    "mcp-agui-endpoint": 7, "mcp-a2a-endpoint": 7, "mcp-embeddings-surrealdb": 7, "agentic-uar-integration": 7,
    "webui-agent-chat": 8, "mcp-embedding-index-lifecycle": 8,
    "mcp-release-binaries": 9,
    "mcp-user-docs": 10, "mcp-plugin-marketplaces": 10, "mcp-ci-workflow": 10,
    "phase-boundary-review": 11,
}


def augment():
    lines = open(TABLE).read().split("\n")
    out = []
    for ln in lines:
        if ln.startswith("| ipfs-mcp-server |"):
            cells = ln.strip().strip("|").split("|")
            cid = cells[1].strip()
            pre = re.sub(r";?\s*after: [^|]*$", "", cells[11].strip())
            deps = DEPS[cid]
            if deps:
                pre = (pre + "; " if pre else "") + "after: " + ", ".join(deps)
            cells[11] = " " + pre + " "
            ln = "|" + "|".join(cells) + "|"
        out.append(ln)
    open(TABLE, "w").write("\n".join(out))


def tasks_of(cid):
    # Both open and completed tasks need a row; matching only "- [ ]" made every ticked task a stale row.
    return re.findall(r"^- \[[ xX]\] (\d+\.\d+) ", open(os.path.join(CH, cid, "tasks.md")).read(), flags=re.M)


def check():
    errors, n = [], 0
    present = set(os.listdir(CH)) - {"archive"}
    if set(ROUND) != present or set(DEPS) != present:
        errors.append(f"change sets differ: round {set(ROUND) ^ present}, deps {set(DEPS) ^ present}")
    rows = {}
    for ln in open(TABLE):
        if ln.startswith("| ipfs-mcp-server |"):
            c = [x.strip() for x in ln.strip().strip("|").split("|")]
            rows[(c[1], c[2])] = c[11]
    for cid in sorted(present):
        for tid in tasks_of(cid):
            n += 1
            pre = rows.get((cid, tid))
            if pre is None:
                errors.append(f"{cid} {tid}: no row")
                continue
            m = re.search(r"after: (.*)$", pre)
            after = [x.strip() for x in m.group(1).split(",")] if m else []
            missing = set(DEPS[cid]) - set(after)
            if missing:
                errors.append(f"{cid} {tid}: prerequisites miss {sorted(missing)}")
            for d in after:
                if d not in ROUND:
                    errors.append(f"{cid} {tid}: unknown prerequisite {d}")
                elif ROUND[d] >= ROUND[cid]:
                    errors.append(f"{cid} {tid}: prerequisite {d} in round {ROUND[d]} >= task round {ROUND[cid]}")
    extra = set(rows) - {(c, t) for c in present for t in tasks_of(c)}
    if extra:
        errors.append(f"stale rows {sorted(extra)}")
    print(f"checked {n} tasks, {len(rows)} rows, {len(present)} changes")
    for e in errors:
        print("FAIL", e)
    print("RESULT", "PASS" if not errors else f"FAIL ({len(errors)})")
    return not errors


def mirror():
    for cid, deps in DEPS.items():
        p = os.path.join(CH, cid, "tasks.md")
        t = open(p).read()
        t = re.sub(r"\nChange-level prerequisites \(plan revision 2\):[^\n]*\n", "\n", t)
        line = ("Change-level prerequisites (plan revision 2): every task in this change also requires completion of: "
                + (", ".join(f"`{d}`" for d in deps) if deps else "none (Round 0)")
                + f". Scheduled in Round {ROUND[cid]}.")
        parts = t.split("\n", 3)  # title, blank, plan reference, rest
        t = "\n".join(parts[:3]) + "\n" + line + "\n" + parts[3]
        open(p, "w").write(t)


augment()
if MIRROR:
    mirror()
sys.exit(0 if check() else 1)
