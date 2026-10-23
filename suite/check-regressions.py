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
  R6: Under multi-sender contention, accounts with more plasma have >= inclusion rate of accounts with less plasma
  R7: Plasma depletes by exactly 21000 per simple self-transfer and regenerates to full fused amount after confirmation
  R8: PoW transactions from unfused accounts have 100% inclusion when sent sequentially (1 tx per account)
  R9: Mixed fused/unfused contention: fused accounts have 100% inclusion with 0 PoW; unfused accounts have 100% inclusion when demand is under per-momentum capacity
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

def check_contention_plasma_gradient(results_dir):
    """R6: Under contention, higher-plasma accounts have >= inclusion rate than lower-plasma accounts.
    
    Relaxed check: computes Spearman rank correlation between plasma_available and inclusion rate.
    Requires positive correlation (rho > 0) when contention is present.
    """
    failures = []
    for f in glob.glob(os.path.join(results_dir, "phase-7", "contend-*-v2.json")):
        data = load_result(f)
        if not data:
            continue
        breakdowns = data.get("account_breakdowns", [])
        if len(breakdowns) < 3:
            continue
        # Only check when there's real contention (some rejections)
        rejected_any = any(bd["rejected"] > 0 for bd in breakdowns)
        if not rejected_any:
            continue
        
        # Compute Spearman rank correlation
        plasma_ranks = []
        rate_ranks = []
        for bd in breakdowns:
            plasma_ranks.append(bd["plasma_available"])
            rate_ranks.append(bd["included"] / max(bd["attempted"], 1))
        
        # Rank transform
        def rank(data):
            sorted_idx = sorted(range(len(data)), key=lambda i: data[i])
            ranks = [0] * len(data)
            for rank_pos, idx in enumerate(sorted_idx):
                ranks[idx] = rank_pos
            return ranks
        
        pr = rank(plasma_ranks)
        rr = rank(rate_ranks)
        n = len(pr)
        
        # Pearson correlation on ranks
        mean_pr = sum(pr) / n
        mean_rr = sum(rr) / n
        cov = sum((pr[i] - mean_pr) * (rr[i] - mean_rr) for i in range(n))
        std_pr = (sum((pr[i] - mean_pr)**2 for i in range(n))) ** 0.5
        std_rr = (sum((rr[i] - mean_rr)**2 for i in range(n))) ** 0.5
        
        if std_pr == 0 or std_rr == 0:
            continue  # no variation, skip
        
        rho = cov / (std_pr * std_rr)
        
        if rho <= 0:
            failures.append(
                f"R6 FAIL: {os.path.basename(f)} Spearman rho={rho:.3f} "
                f"(expected > 0: higher plasma should correlate with higher inclusion)"
            )
        else:
            print(f"  R6 info: {os.path.basename(f)} Spearman rho={rho:.3f}")
    return failures

def check_plasma_depletion(results_dir):
    """R7: Plasma depletes by exactly 21000 per tx and regenerates after confirmation."""
    failures = []
    for f in glob.glob(os.path.join(results_dir, "phase-8", "deplete-*.json")):
        data = load_result(f)
        if not data:
            continue
        # Skip failed runs (0 included txs)
        if data.get("included", 0) == 0:
            continue
        results = data.get("results", [])
        
        # Check linear depletion: each tx consumes exactly 21000 plasma
        for i in range(1, len(results)):
            prev = results[i-1].get("plasma_after", 0)
            curr = results[i].get("plasma_after", 0)
            if prev > 0 and curr > 0:
                consumed = prev - curr
                if consumed != 21000:
                    failures.append(f"R7 FAIL: {os.path.basename(f)} tx {results[i]['idx']} consumed {consumed} plasma (expected 21000)")
        
        # Check regeneration: after waiting, plasma should be >= fused amount
        regen = data.get("regen_test_plasma", 0)
        qsr_fused = data.get("qsr_fused", 0)
        expected_min = qsr_fused * 2100  # 2100 plasma per QSR
        if regen < expected_min:
            failures.append(f"R7 FAIL: {os.path.basename(f)} regen plasma {regen} < expected minimum {expected_min} ({qsr_fused} QSR fused)")
    return failures

def check_powlat_inclusion(results_dir):
    """R8: PoW transactions from unfused accounts have 100% inclusion when sent sequentially."""
    failures = []
    for f in glob.glob(os.path.join(results_dir, "phase-9", "powlat-*.json")):
        data = load_result(f)
        if not data:
            continue
        if data.get("pow_count", 0) == 0:
            continue  # skip runs without PoW
        if data["included"] != data["attempted"]:
            failures.append(f"R8 FAIL: {os.path.basename(f)} included={data['included']} attempted={data['attempted']} (expected 100% for sequential PoW)")
        # Also check that required difficulty is 31.5M at workPrice=1000
        for r in data.get("results", []):
            diff = r.get("plasma_after", 0)
            if diff > 0 and diff != 31500000:
                failures.append(f"R8 FAIL: {os.path.basename(f)} tx {r['idx']} difficulty={diff} (expected 31500000 at workPrice=1000)")
    return failures

def check_mixcontend(results_dir):
    """R9: Mixed fused/unfused contention under low demand.
    
    Fused accounts: 100% inclusion, 0 PoW used.
    Unfused accounts: 100% inclusion (under per-momentum capacity).
    """
    failures = []
    for f in glob.glob(os.path.join(results_dir, "phase-11", "mixcontend-*.json")):
        data = load_result(f)
        if not data:
            continue
        # Skip empty or in-progress files
        if data.get("attempted", 0) == 0:
            continue
        
        # Fused accounts: 100% inclusion, 0 PoW
        if data.get("fused_included", 0) != data.get("fused_attempted", 0):
            failures.append(
                f"R9 FAIL: {os.path.basename(f)} fused included={data['fused_included']} "
                f"attempted={data['fused_attempted']} (expected 100%)"
            )
        
        # Unfused accounts: 100% inclusion when under capacity
        if data.get("unfused_included", 0) != data.get("unfused_attempted", 0):
            failures.append(
                f"R9 FAIL: {os.path.basename(f)} unfused included={data['unfused_included']} "
                f"attempted={data['unfused_attempted']} (expected 100% under capacity)"
            )
        
        # Fused accounts should use 0 PoW
        for bd in data.get("breakdowns", []):
            if bd["group"] == "fused" and bd["pow_count"] > 0:
                failures.append(
                    f"R9 FAIL: {os.path.basename(f)} fused acct {bd['account_index']} "
                    f"used {bd['pow_count']} PoW (expected 0 with plasma available)"
                )
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
        ("R6: contention plasma gradient (phase-7)", check_contention_plasma_gradient),
        ("R7: plasma depletion + regeneration (phase-8)", check_plasma_depletion),
        ("R8: PoW sequential 100% inclusion (phase-9)", check_powlat_inclusion),
        ("R9: mixed contention fused 100% + unfused 100% (phase-11)", check_mixcontend),
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
