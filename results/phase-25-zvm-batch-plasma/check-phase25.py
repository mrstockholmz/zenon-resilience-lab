#!/usr/bin/env python3
"""Phase 25 regression assertions: ZVM devnet L1 batch-plasma measurements.

Usage: python3 check-phase25.py [results_dir]

PASS/FAIL/NOT TESTED per assertion, lab style. Reads:
  phase-25-zvm-batch-plasma/probe-result.json   (size ladder)
  phase-25-zvm-batch-plasma/burst-result.json   (blocks-per-momentum binding)
"""
import json
import os
import sys

NOT_TESTED, FAIL, PASS = "NOT TESTED", "FAIL", "PASS"


def check_ladder(d):
    path = os.path.join(d, "probe-result.json")
    if not os.path.exists(path):
        return NOT_TESTED, ["probe-result.json missing"]
    s = json.load(open(path))
    problems, checked = [], 0
    for r in s.get("sizes", []):
        size = r["data_size_bytes"]
        planned = 21000 + 68 * size
        if size > 16384:
            if not r.get("rejected_over_cap") and not r.get("publish_error"):
                problems.append(f"size {size}: expected rejection over 16 KiB cap")
            continue
        if not r.get("send_hash"):
            problems.append(f"size {size}: no send hash (publish_error={r.get('publish_error')!r})")
            continue
        checked += 1
        if r.get("used_plasma") != planned:
            problems.append(
                f"size {size}: used_plasma={r.get('used_plasma')} != planned {planned}"
            )
    if checked == 0:
        return NOT_TESTED, ["no included ladder sends to check"]
    if problems:
        return FAIL, problems
    return PASS, [f"{checked} ladder sizes match 21000+68*B exactly"]


def check_burst(d):
    path = os.path.join(d, "burst-result.json")
    if not os.path.exists(path):
        return NOT_TESTED, ["burst-result.json missing"]
    s = json.load(open(path))
    blocks = s.get("blocks", [])
    counts = s.get("per_momentum_counts", {})
    problems = []
    included = [b for b in blocks if b.get("confirming_momentum")]
    if len(included) < len(blocks):
        problems.append(f"only {len(included)}/{len(blocks)} blocks confirmed")
    if len(blocks) >= 8:
        max_per_mom = max(counts.values()) if counts else 0
        # protocol budget 4,200,000 / 578,056 (8 KiB block) = 7.26 -> at most 7
        if max_per_mom > 7:
            problems.append(
                f"{max_per_mom} blocks in one momentum; expected <= 7 (4.2M budget / 578,056)"
            )
        if len(counts) < 2:
            problems.append(
                f"all {len(blocks)} blocks in one momentum; expected spread over >= 2 "
                "(9 blocks x 578,056 = 5.2M > 4.2M budget)"
            )
    for b in included:
        if b.get("used_plasma") != 21000 + 68 * b.get("size_bytes", 0):
            problems.append(f"block {b.get('hash','?')[:12]}: used_plasma mismatch")
    if problems:
        return FAIL, problems
    return PASS, [
        f"{len(included)} blocks confirmed, per-momentum counts={counts} "
        f"(max {max(counts.values()) if counts else 0} <= 7)"
    ]


def main():
    results_dir = sys.argv[1] if len(sys.argv) > 1 else os.path.dirname(os.path.abspath(__file__))
    checks = {
        "R21a ladder formula 21000+68*B on ZVM L1": check_ladder,
        "R21b <=7 x 8KiB blocks per momentum (budget binding)": check_burst,
    }
    overall = True
    for name, fn in checks.items():
        status, detail = fn(results_dir)
        overall = overall and status == PASS
        print(f"{status}: {name} -- {' ; '.join(detail)}")
    sys.exit(0 if overall else 1)


if __name__ == "__main__":
    main()
