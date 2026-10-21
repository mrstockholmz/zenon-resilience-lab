# Phase 4 Results — Static vs Dynamic Plasma Comparison (Fused Accounts)

**Date:** 2026-10-22
**Node:** lab devnet, go-zenon `e89f91f`, single pillar (1 of 3), chain 69
**Regimes:** Static (height < 30) and Dynamic (height >= 30)
**Spork:** dynamic-plasma at enforcementHeight 30

## Design

Phase 3 established that fused plasma eliminates PoW and enables 100% inclusion
for both sequential and burst workloads in the dynamic regime. Phase 4 asks:
does the Dynamic Plasma pricing mechanism (activated at height 30) change
anything for accounts that already have fused plasma?

The key difference between the regimes:
- **Static (height < 30):** `nextFusionPrice = 0`, `nextWorkPrice = 0`. Plasma
  pricing is fixed; fusion and PoW have no dynamic cost adjustment.
- **Dynamic (height >= 30):** `nextFusionPrice = 1000`, `nextWorkPrice = 1000`.
  Prices are non-zero and adjust based on momentum-level plasma usage.

Tests use the same agent account (z1qp3yph55qgresyytz83anynr2f4z39x2z3ej3e)
with 100 QSR fused (210000 max plasma) throughout.

## Results

### high-steady-1 (fused plasma, 5 sequential self-transfers)

| regime | seed | included | PoW count | total PoW ms | p50 ms | duration s |
|--------|------|----------|-----------|--------------|--------|------------|
| static | 0 | 5/5 (100%) | 4* | 99792 | 0 | 99.8 |
| static | 1 | 5/5 (100%) | 1* | 24900 | 0 | 24.9 |
| static | 2 | 5/5 (100%) | 2* | 20558 | 1 | 20.6 |
| dynamic | 0 | 5/5 (100%) | 0 | 0 | 0 | 0.018 |
| dynamic | 1 | 5/5 (100%) | 0 | 0 | 0 | 0.021 |
| dynamic | 2 | 5/5 (100%) | 0 | 0 | 0 | 0.019 |

*Static regime seeds 0-2 include the initial fuse transaction and early
transactions before plasma was available. The fuse itself required PoW
(18-40s). Once plasma was available, subsequent transactions used 0 PoW.

**Dynamic regime note:** In the dynamic regime, the agent had already fused
plasma (from the static-regime tests). No refusion was needed, so all 5
transactions per seed ran with 0 PoW and near-zero latency.

**Key finding:** Once plasma is fused, there is no difference between static
and dynamic regimes for sequential transactions. Both achieve 100% inclusion
with 0 PoW and ~0ms latency. The dynamic pricing mechanism does not affect
the per-transaction plasma cost for simple transfers.

### high-burst-1 (fused plasma, 5 parallel self-transfers)

| regime | seed | included | PoW count | total PoW ms | p50 ms | duration s |
|--------|------|----------|-----------|--------------|--------|------------|
| static | 0 | 5/5 (100%) | 0 | 0 | 0 | 0.052 |
| static | 1 | 5/5 (100%) | 0 | 0 | 1 | 0.053 |
| static | 2 | 5/5 (100%) | 0 | 0 | 1 | 0.053 |
| dynamic | 0 | 5/5 (100%) | 0 | 0 | 0 | 0.052 |
| dynamic | 1 | 5/5 (100%) | 0 | 0 | 0 | 0.053 |
| dynamic | 2 | 5/5 (100%) | 0 | 0 | 0 | 0.054 |

**Key finding:** Burst mode with fused plasma is identical in both regimes:
100% inclusion, 0 PoW, ~0.05s total duration. The 10ms stagger between
goroutine starts is sufficient for each transaction to build on the updated
frontier when there is no PoW delay.

### high-burst-multi (2 accounts, 3 parallel self-transfers each)

| regime | seed | included | PoW count | total PoW ms | duration s |
|--------|------|----------|-----------|--------------|------------|
| static | 0-r2 | 6/6 (100%) | 3** | 50624 | 50.6 |
| dynamic | 0 | 6/6 (100%) | 3** | 54748 | 54.8 |

**Account 1** (20000 QSR genesis, ~42M plasma): 3/3 included, 0 PoW, instant.
**Account 2** (0 fused plasma, PoW fallback): 3/3 included, 3 PoW (15-33s each).

**Both regimes identical:** Account 1 with massive plasma sends instantly.
Account 2 without plasma falls back to PoW with the same cost in both regimes.

*Initial attempt (static seed0) failed because account 2 tried to send before
the funding transaction was received. After running `labctl receive 2`, the
retry succeeded. This is a test setup race, not a protocol finding.*

## Comparison Summary

| metric | static (fused) | dynamic (fused) | difference |
|--------|---------------|-----------------|------------|
| steady inclusion | 100% (15/15) | 100% (15/15) | none |
| steady PoW (available plasma) | 0 | 0 | none |
| steady latency (p50) | 0-1 ms | 0 ms | none |
| burst inclusion | 100% (15/15) | 100% (15/15) | none |
| burst duration | 0.052-0.053 s | 0.052-0.054 s | none |
| fusion price | 0 | 1000 | cosmetic* |
| work price | 0 | 1000 | cosmetic* |

*The price fields are non-zero in dynamic regime but do not affect transaction
cost for simple transfers at low volume. Dynamic Plasma repricing only fires
when momentum-level plasma usage strictly exceeds the target (confirmed in
phase 2 and in the go-zenon code).

## Conclusion

**For accounts with sufficient fused plasma, there is no observable difference
between static and dynamic plasma regimes.** The Dynamic Plasma pricing
mechanism is designed to adjust costs under high demand (when momentum-level
plasma usage exceeds the target). At the low volumes tested here (5-10 simple
transfers per block), the repricing never triggers.

This means:
1. Fused plasma holders are insulated from Dynamic Plasma pricing at normal
   usage levels.
2. The key benefit of fused plasma (0 PoW, instant inclusion) is identical
   in both regimes.
3. Dynamic Plasma primarily affects *new* fusions and *PoW fallback* costs
   under high demand, not existing fusion holders at low volume.

## Raw Data

- `static-high-steady-1-seed{0,1,2}.json`
- `static-high-burst-1-seed{0,1,2}.json`
- `static-high-burst-multi-seed0.json` (failed: unfunded account)
- `static-high-burst-multi-seed0-r2.json` (after receive)
- `dynamic-high-steady-1-seed{0,1,2}.json`
- `dynamic-high-burst-1-seed{0,1,2}.json`
- `dynamic-high-burst-multi-seed0.json`
