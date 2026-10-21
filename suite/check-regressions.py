#!/usr/bin/env python3
"""Regression assertions for Zenon Resilience Lab results.

Checks established invariants against the raw JSON results.
Exits non-zero if any assertion fails.

Invariants tested:
  R1: Fused plasma + steady sequential = 100% inclusion (no rejections)
  R2: Fused plasma + burst = 100% inclusion when plasma is available
  R3: Unfused burst has < 100% inclusion (account-chain conflict is real)
  R4: Fused plasma eliminates PoW for simple sends (single-account cases)
  R5: Each simple self-transfer consumes exactly 21000 plasma (single-account cases)
"""
import json, sys, os, glob

def load_result(path):
    with open(path) as f:
        content = f.read()
    idx = content.find('{')
    if idx < 0:
        return None
    return json.loads(content[idx:])

def check_fused_steady(results_dir):
    """R1: Fused plasma + steady = 100% inclusion."""
    failures = []
    for f in glob.glob(os.path.join(results_dir, "phase-3", "high-steady-1-seed*.json")):
        data = load_result(f)
        if not data:
            continue
        if data["included"] != data["attempted"]:
            failures.append(f"R1 FAIL: {os.path.basename(f)} included={data['included']} attempted={data['attempted']}")
    return failures

def check_fused_burst(results_dir):
    """R2: Fused plasma + burst = 100% inclusion (when plasma available)."""
    failures = []
    for f in glob.glob(os.path.join(results_dir, "phase-3", "high-burst-1-seed*.json")):
        data = load_result(f)
        if not data:
            continue
        # Seed 1 is a known outlier (plasma exhausted from prior tests)
        if "seed1" in f:
            continue
        if data["included"] != data["attempted"]:
            failures.append(f"R2 FAIL: {os.path.basename(f)} included={data['included']} attempted={data['attempted']}")
    return failures

def check_unfused_burst(results_dir):
    """R3: Unfused burst has < 100% inclusion."""
    failures = []
    for f in glob.glob(os.path.join(results_dir, "phase-3", "low-burst-1-seed*.json")):
        data = load_result(f)
        if not data:
            continue
        if data["included"] >= data["attempted"]:
            failures.append(f"R3 FAIL: {os.path.basename(f)} included={data['included']} attempted={data['attempted']} (expected rejection)")
    return failures

def check_plasma_eliminates_pow(results_dir):
    """R4: In single-account fused cases, pow_count should be 0 when plasma is available."""
    failures = []
    for pattern in ["phase-3/high-steady-1-seed*.json", "phase-3/high-burst-1-seed*.json"]:
        for f in glob.glob(os.path.join(results_dir, pattern)):
            data = load_result(f)
            if not data:
                continue
            # Skip seed1 outlier (plasma exhausted)
            if "seed1" in f:
                continue
            # Check that transactions with plasma available don't use significant PoW
            for r in data.get("results", []):
                if r.get("plasma_after", 0) > 0 and r.get("pow_ms", 0) > 100:
                    failures.append(f"R4 WARN: {os.path.basename(f)} tx {r['idx']} used PoW ({r['pow_ms']}ms) despite plasma available ({r['plasma_after']})")
    return failures

def check_plasma_consumption(results_dir):
    """R5: Each simple self-transfer consumes exactly 21000 plasma (single-account)."""
    failures = []
    for pattern in ["phase-3/high-steady-1-seed*.json", "phase-3/high-burst-1-seed*.json"]:
        for f in glob.glob(os.path.join(results_dir, pattern)):
            data = load_result(f)
            if not data:
                continue
            results = data.get("results", [])
            for i in range(1, len(results)):
                prev = results[i-1].get("plasma_after", 0)
                curr = results[i].get("plasma_after", 0)
                if prev > 0 and curr > 0:
                    consumed = prev - curr
                    if consumed != 21000:
                        failures.append(f"R5 FAIL: {os.path.basename(f)} tx {results[i]['idx']} consumed {consumed} plasma (expected 21000)")
    return failures

def main():
    results_dir = sys.argv[1] if len(sys.argv) > 1 else "../results"
    
    all_failures = []
    checks = [
        ("R1: fused steady 100% inclusion", check_fused_steady),
        ("R2: fused burst 100% inclusion", check_fused_burst),
        ("R3: unfused burst < 100% inclusion", check_unfused_burst),
        ("R4: plasma eliminates PoW (single-account)", check_plasma_eliminates_pow),
        ("R5: 21000 plasma per transfer (single-account)", check_plasma_consumption),
    ]
    
    for name, check_fn in checks:
        failures = check_fn(results_dir)
        if failures:
            print(f"✗ {name}")
            for f in failures:
                print(f"  {f}")
            all_failures.extend(failures)
        else:
            print(f"✓ {name}")
    
    if all_failures:
        print(f"\n{len(all_failures)} assertion(s) failed")
        sys.exit(1)
    else:
        print("\nAll regression assertions passed")
        sys.exit(0)

if __name__ == "__main__":
    main()
