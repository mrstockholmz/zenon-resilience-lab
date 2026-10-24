# Phase 21: Momentum Production Rate Analysis

**Date:** 2026-10-25
**Question:** What is the actual momentum production rate on the lab devnet, and how does it compare to the theoretical 10-second target? Does transaction load affect the production rate?
**Method:** Analyzed 8,646 inter-momentum deltas from the devnet log (heights 2–8648), plus two live collection runs (baseline and under burst load). Compared observed distribution with theoretical model from the go-zenon election algorithm.

## Setup

- 3 pillars in genesis (dev1, dev2, dev3), only dev1 running
- BlockTime = 10s, NodeCount = 3 → each tick = 30s with 3 slots
- Election: all 3 pillars selected every tick, order shuffled deterministically by momentum height
- Momentum timestamp = slot start time (deterministic from genesis), NOT wall-clock time
- Missed slots (offline pillar) are skipped entirely — no catch-up mechanism
- 3-second too-late guard: momentum not broadcast if produced >3s past slot start

## Key Findings

### 1. Mean inter-momentum time is 30.0s (3× slowdown)

With 3 pillars and only 1 active, the expected rate is 30s/momentum (only 1/3 of slots are produced). The observed mean over 8,646 samples is exactly 30.0s, confirming the structural slowdown.

| Metric | Value |
|--------|-------|
| Mean | 30.0s |
| Median | 20s |
| Stddev | 22.4s |
| Min | 10s |
| Max | 180s |
| Effective rate | 30.0s/momentum |
| Slowdown vs 10s target | 3.0× |

### 2. Distribution does NOT match uniform shuffle model

Theoretical distribution (uniform shuffle, 3 pillars, 1 active):

| Delta | Predicted | Observed | Difference |
|-------|-----------|----------|------------|
| 10s | 11.1% | 30.9% | +19.8pp |
| 20s | 22.2% | 22.2% | ±0 |
| 30s | 33.3% | 15.8% | −17.5pp |
| 40s | 22.2% | 11.1% | −11.1pp |
| 50s | 11.1% | 7.3% | −3.8pp |
| 60s+ | 0% | 12.7% | +12.7pp |

The observed distribution has significantly more 10s deltas (30.9% vs 11.1%) and more long gaps (>60s) than the uniform model predicts. This suggests:

1. **The deterministic shuffle (math/rand with momentum height seed) is biased** toward consecutive slots for the same pillar when the permutation space is tiny (3! = 6 permutations).
2. **Slot misses occur**: when the node is slow to produce (system load, I/O delay), the 3-second too-late guard drops the momentum, creating longer gaps.

### 3. Momentum timestamp = slot start time (0s delay)

Analysis of 50 momentum production events shows the momentum timestamp exactly matches the wall-clock time (0s delay in all cases, at 1-second log granularity). The timestamp is the deterministic slot start time, not the actual production time.

### 4. Transaction load does NOT significantly affect production rate

| Condition | Mean dt | Sample size |
|-----------|---------|-------------|
| Baseline (no load) | 20.8s | 12 deltas |
| Burst load (100 txs) | 18.8s | 8 deltas |
| Funding load (50 txs) | 40.0s | 6 deltas |

The burst load (100 self-transfers, 90 included in one momentum) shows a similar rate to baseline. The funding load test had a small sample and was affected by a fusion failure (insufficient QSR), making it inconclusive. No evidence that transaction load slows momentum production.

### 5. Implications for previous lab results

All previous phases measured per-momentum behavior, which is **correct and unaffected** by the production rate. However:

- **Per-second throughput calculations are 3× lower** than per-momentum numbers suggest. The effective throughput is ~6.7 txs/second (200 txs / 30s), not 20 txs/second.
- **Latency measurements in wall-clock time** are affected by the irregular production rate. A transaction submitted just after a momentum might wait 10–180s for the next one.
- **The 10s momentum period documented in go-zenon constants** assumes all pillars are online. On a devnet with offline pillars, the effective period scales by NodeCount/ActiveCount.

## Theoretical Model

With N pillars and K active (K ≤ N), the expected inter-momentum time is:

```
E[dt] = BlockTime × N / K
```

For the lab devnet: 10s × 3/1 = 30s ✓ (observed: 30.0s)

The distribution depends on the shuffle algorithm. With Go's `math/rand.Perm(3)`, the 6 permutations are not uniformly distributed over small seeds, leading to the observed bias toward consecutive slots.

## Raw Data

- `results/phase-21/momentum-rate-baseline.json` — 5-minute baseline collection (13 momentums)
- `results/phase-21/momentum-rate-burst.json` — 3-minute burst load collection (9 momentums)
- `results/phase-21/momentum-rate-load-50acct-4tx-seed1792884528.json` — funding load collection (7 momentums)
- Historical log analysis: 8,646 inter-momentum deltas from `/home/mrstockholmez/lab-devnet/znnd.log`

## Regression Assertion R18

**R18: Momentum production rate with N pillars and K active shall equal BlockTime × N/K.**

On the lab devnet (N=3, K=1, BlockTime=10s), the mean inter-momentum time shall be 30s ± 20%. Observed: 30.0s (PASS).

## Source References

| What | File:Line |
|------|-----------|
| Timestamp = slot start time | `pillar/worker_momentum.go:42,54` |
| Slot start time computation | `consensus/election.go:41-59` |
| Tick = BlockTime × NodeCount | `consensus/context.go:31` |
| BlockTime=10, NodeCount=30 (mainnet) | `vm/constants/consensus.go:28-30` |
| Election: weighted + random selection | `consensus/election_algorithm.go:53-130` |
| Shuffle: math/rand.Perm with height seed | `consensus/election_algorithm.go:64-73` |
| Pillar only produces if event names its coinbase | `pillar/manager.go:95-96` |
| Too-late guard (3s past slot start) | `pillar/worker.go:110-111` |
| Missed slot = skipped (no catch-up) | `consensus/consensus.go:212-213` |
