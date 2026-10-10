#!/usr/bin/env python3
"""Offline trigger check for mcp/skills.

Scores each should-trigger query in evals/trigger-evals.json against every
skill's description with a token-overlap measure (IDF weighted) and reports
whether the best-scoring skill is the expected one (routing between the
skills of this set). Near-miss queries (expect=null) are listed but not
scored.

This is a deterministic lexical proxy for "would the description match",
NOT a live model trigger test: it ignores how a given agent decides to
consult a skill. Run live trigger evaluation separately when a harness is
available.

Usage: python3 mcp/skills/scripts/trigger_check.py [--min-accuracy 0.9]
"""
import argparse
import json
import math
import re
import sys
from collections import Counter
from pathlib import Path

ROOT = Path(__file__).resolve().parent.parent
STOP = set(
    "a an and are as at be but by can do for from has have how i if in into is it its me my "
    "of on or so that the their this to up use was what when whenever which who will with you your "
    "even if they never name tool tools mcp skill ipfs node".split()
)
THRESHOLD = 0.08


def tokens(text):
    out = []
    for w in re.findall(r"[a-z0-9]+", text.lower().replace("_", " ")):
        if w in STOP or len(w) < 3:
            continue
        out.append(w[:-1] if w.endswith("s") and len(w) > 4 else w)
    return out


def description(path):
    m = re.search(r"^description:\s*(.+)$", path.read_text(), re.M)
    return m.group(1) if m else ""


def main():
    ap = argparse.ArgumentParser()
    ap.add_argument("--min-accuracy", type=float, default=0.9)
    args = ap.parse_args()
    skills = {p.parent.name: Counter(tokens(description(p))) for p in sorted(ROOT.glob("*/SKILL.md"))}
    df = Counter(w for c in skills.values() for w in c)
    n = len(skills)
    idf = {w: math.log((n + 1) / (df[w] + 0.5)) for w in df}
    cases = json.loads((ROOT / "evals" / "trigger-evals.json").read_text())
    correct = 0
    negatives = [c for c in cases if c["expect"] is None]
    cases = [c for c in cases if c["expect"] is not None]
    for case in cases:
        q = Counter(tokens(case["query"]))
        scores = {}
        for name, c in skills.items():
            hit = sum(idf[w] * min(q[w], 1) for w in q if w in c)
            norm = math.sqrt(sum(idf[w] ** 2 for w in q if w in idf)) or 1.0
            scores[name] = hit / (norm * 3)
        best = max(scores, key=scores.get)
        got = best if scores[best] >= THRESHOLD else None
        ok = got == case["expect"]
        correct += ok
        print(("PASS" if ok else "FAIL"), f"expect={case['expect']} got={got} ({scores[best]:.2f})  {case['query'][:70]}")
    acc = correct / len(cases)
    print(f"routing accuracy {correct}/{len(cases)} = {acc:.2f}")
    print(
        f"{len(negatives)} near-miss queries (expect=null) are not scored here: a lexical proxy cannot "
        "tell 'pin a dependency' from 'pin a CID'. They need a live model trigger run."
    )
    return 0 if acc >= args.min_accuracy else 1


if __name__ == "__main__":
    sys.exit(main())
