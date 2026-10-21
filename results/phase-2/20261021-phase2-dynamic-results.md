# Phase 2 Results — Dynamic Plasma Regime (momentum >= 30)

**Date:** 2026-10-21
**Node:** lab devnet, go-zenon `e89f91f`, single pillar, chain 69
**Regime:** Dynamic Plasma (momentum height >= 30)
**Spork activated:** "dynamic-plasma" at height 30 (from node log)

## Ordinary Workload (sequential self-transfers, 1 ZNN each)

| run | attempted | submitted | included | rejected | PoW count | total PoW ms | p50 lat | duration s |
|-----|-----------|-----------|----------|----------|-----------|--------------|---------|------------|
| r1  | 3         | 3         | 3        | 0        | 3         | 50427        | 1       | 50.4       |
| r2  | 3         | 3         | 3        | 0        | 3         | 7309         | 1       | 7.3        |
| r3  | 3         | 3         | 3        | 0        | 3         | 34300        | 1       | 34.3       |

## Burst Workload (5 parallel self-transfers from single account)

| run | attempted | submitted | included | rejected | inclusion rate |
|-----|-----------|-----------|----------|----------|----------------|
| r1  | 5         | 2         | 2        | 3        | 40%            |
| r2  | 5         | 2         | 2        | 3        | 40%            |
| r3  | 5         | 2         | 2        | 3        | 40%            |

## Probe (lightly-provisioned account, 10s interval)

| run | attempted | submitted | included | rejected | PoW count | total PoW ms |
|-----|-----------|-----------|----------|----------|-----------|--------------|
| r1  | 2         | 2         | 2        | 0        | 2         | 56821        |

## Comparison: Static vs Dynamic (unfused accounts)

| metric | static (r1-r3 avg) | dynamic (r1-r3 avg) |
|--------|-------------------|---------------------|
| ordinary inclusion | 100% (9/9) | 100% (9/9) |
| ordinary PoW ms (avg total) | 27236 | 30679 |
| burst inclusion | 33% (5/15) | 40% (6/15) |
| burst PoW ms (avg total) | 15414 | 15254 |

**Finding:** For unfused accounts (PoW fallback), there is no significant difference
between static and dynamic plasma regimes in terms of inclusion rate or PoW cost.
This is expected: Dynamic Plasma primarily affects the *pricing* of fused plasma
and the difficulty adjustment of PoW, but for a single account making simple transfers,
the base plasma requirement is the same.

The dynamic plasma pricing (nextFusionPrice=1000, nextWorkPrice=1000) only kicks in
when momentum-level plasma usage exceeds the target, which doesn't happen with our
low-volume tests.

## Raw Data

- `dynamic-ordinary-r1.json`, `dynamic-ordinary-r2.json`, `dynamic-ordinary-r3.json`
- `dynamic-burst-r1.json`, `dynamic-burst-r2.json`, `dynamic-burst-r3.json`
- `dynamic-probe-r1.json`
