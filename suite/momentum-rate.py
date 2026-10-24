#!/usr/bin/env python3
"""Phase 21: Momentum production rate analysis.

Measures the actual inter-momentum time distribution on the lab devnet,
comparing momentum-embedded timestamps with wall-clock observation times.

Usage: python3 momentum-rate.py [--duration SECS] [--out FILE]
"""

import json
import time
import sys
import argparse
import urllib.request
from datetime import datetime

RPC = "http://127.0.0.1:57997"


def rpc(method, params=None):
    req = urllib.request.Request(
        RPC,
        data=json.dumps({"jsonrpc": "2.0", "id": 1, "method": method, "params": params or []}).encode(),
        headers={"Content-Type": "application/json"},
    )
    resp = json.loads(urllib.request.urlopen(req, timeout=15).read())
    if "error" in resp:
        raise Exception(f"RPC error: {resp['error']}")
    return resp["result"]


def get_momentum_by_height(height):
    try:
        result = rpc("ledger.getMomentumsByHeight", [height, 1])
        if result and "list" in result and len(result["list"]) > 0:
            return result["list"][0]
    except Exception:
        pass
    return None


def main():
    parser = argparse.ArgumentParser(description="Momentum production rate analysis")
    parser.add_argument("--duration", type=int, default=300, help="Collection duration in seconds")
    parser.add_argument("--out", type=str, default=None, help="Output JSON file")
    args = parser.parse_args()

    print(f"[momentum-rate] Collecting for {args.duration}s...")
    start_wall = time.time()
    start_height = rpc("ledger.getFrontierMomentum")["height"]
    print(f"[momentum-rate] Starting at height {start_height}")

    samples = []
    last_height = start_height
    last_ts = None

    while time.time() - start_wall < args.duration:
        try:
            frontier = rpc("ledger.getFrontierMomentum")
            h = frontier["height"]
            ts = frontier["timestamp"]
            wall = time.time()

            if h > last_height:
                # Fetch each new momentum
                for bh in range(last_height + 1, h + 1):
                    m = get_momentum_by_height(bh)
                    if m:
                        samples.append({
                            "height": bh,
                            "momentum_ts": m["timestamp"],
                            "observed_wall": wall,
                            "num_txs": len(m.get("content", [])),
                        })
                        if last_ts is not None:
                            dt = m["timestamp"] - last_ts
                            print(f"  height={bh} dt={dt}s txs={len(m.get('content', []))}")
                        else:
                            print(f"  height={bh} (first) txs={len(m.get('content', []))}")
                        last_ts = m["timestamp"]
                last_height = h
        except Exception as e:
            print(f"  error: {e}", file=sys.stderr)
        time.sleep(1)

    end_wall = time.time()
    elapsed = end_wall - start_wall
    end_height = last_height
    total_momentums = len(samples)

    print(f"\n[momentum-rate] Collected {total_momentums} momentums in {elapsed:.1f}s")
    print(f"[momentum-rate] Height range: {start_height} -> {end_height}")

    if total_momentums < 2:
        print("Not enough data")
        return

    # Compute inter-momentum deltas
    deltas = []
    for i in range(1, len(samples)):
        dt = samples[i]["momentum_ts"] - samples[i - 1]["momentum_ts"]
        deltas.append(dt)

    # Also compute wall-clock deltas
    wall_deltas = []
    for i in range(1, len(samples)):
        wdt = samples[i]["observed_wall"] - samples[i - 1]["observed_wall"]
        wall_deltas.append(round(wdt, 3))

    # Analysis
    deltas_sorted = sorted(deltas)
    n = len(deltas)
    mean_dt = sum(deltas) / n
    median_dt = deltas_sorted[n // 2] if n % 2 == 1 else (deltas_sorted[n // 2 - 1] + deltas_sorted[n // 2]) / 2
    min_dt = min(deltas)
    max_dt = max(deltas)

    # Distribution buckets
    from collections import Counter
    dist = Counter(deltas)

    # Rate calculations
    total_ts_span = samples[-1]["momentum_ts"] - samples[0]["momentum_ts"]
    effective_rate = total_ts_span / (total_momentums - 1) if total_momentums > 1 else 0

    result = {
        "config": {
            "duration_requested": args.duration,
            "duration_actual": round(elapsed, 1),
            "start_height": start_height,
            "end_height": end_height,
            "total_momentums": total_momentums,
        },
        "inter_momentum_deltas": deltas,
        "wall_clock_deltas": wall_deltas,
        "statistics": {
            "mean": round(mean_dt, 2),
            "median": round(median_dt, 2),
            "min": min_dt,
            "max": max_dt,
            "stddev": round((sum((d - mean_dt) ** 2 for d in deltas) / n) ** 0.5, 2),
            "effective_seconds_per_momentum": round(effective_rate, 2),
            "theoretical_seconds_per_momentum": 10,
            "slowdown_factor": round(effective_rate / 10, 2),
        },
        "distribution": {str(k): v for k, v in sorted(dist.items())},
        "samples": samples,
    }

    print(f"\n=== Momentum Production Rate Analysis ===")
    print(f"  Momentums collected: {total_momentums}")
    print(f"  Time span (momentum ts): {total_ts_span}s")
    print(f"  Mean inter-momentum time: {mean_dt:.1f}s")
    print(f"  Median: {median_dt:.1f}s")
    print(f"  Min: {min_dt}s, Max: {max_dt}s")
    print(f"  Stddev: {result['statistics']['stddev']}s")
    print(f"  Effective rate: {effective_rate:.1f}s/momentum")
    print(f"  Slowdown vs 10s target: {effective_rate / 10:.1f}x")
    print(f"\n  Delta distribution:")
    for k, v in sorted(dist.items()):
        pct = v / n * 100
        bar = "#" * int(pct / 2)
        print(f"    {k:3d}s: {v:3d} ({pct:5.1f}%) {bar}")

    if args.out:
        with open(args.out, "w") as f:
            json.dump(result, f, indent=2)
        print(f"\nResults written to {args.out}")


if __name__ == "__main__":
    main()
