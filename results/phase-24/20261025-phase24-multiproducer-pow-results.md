# Phase 24: Multi-Producer PoW Latency — 10s Momenta

**Date:** 2026-10-25
**Devnet:** chain 69, go-zenon `e89f91f`, 3 pillars all active (10s momenta)
**Previous phases:** Phase 9 (PoW latency, 1-producer, 30s momenta), Phase 23 (multi-producer baseline)

## Question

With 3 active pillars producing momenta every 10s (vs 30s in single-producer),
do PoW transactions still achieve same-momentum inclusion? The phase-23 report
hypothesized that PoW txs taking 30-60s to compute might miss multiple momenta
when momenta are 10s apart. This phase tests that hypothesis empirically.

## Method

### Experiment 1: PoW Latency (powlat)

`labctl powlat <n> 1 <baseIdx>` — n unfused accounts (0 plasma) each send
1 self-transfer with PoW, all in parallel. Measures PoW computation time and
inclusion latency.

- Seed 0: accounts 2000-2004 (5 accounts, fresh)
- Seed 1: accounts 2013-2017 (5 accounts, fresh)
- Seed 2: accounts 2010-2012 (3 accounts, fresh)

### Experiment 2: Mixed Fused/Unfused Contention (mixcontend)

`labctl mixcontend 3 3 2 1 100 2026 skipfusion` — 3 fused accounts (100-102,
existing plasma ~1.4M each) + 3 unfused accounts (2026-2028, 0 plasma, 5 ZNN each),
each sending 2 self-transfers of 1 ZNN simultaneously.

**Multi-producer config:** 3/3 active pillars, 10.0s mean inter-momentum time
(phase 23 configuration).

## Key Findings

### 1. PoW Inclusion Rate: 100% (13/13 across 3 powlat seeds)

All PoW transactions were included, regardless of momentum rate. The mempool
holds transactions across momenta, so even if a PoW tx takes 60s to compute
(6 momentum periods at 10s each), it is still included once submitted.

### 2. Same-Momentum Inclusion: 100% for powlat

Every PoW tx in the powlat experiment was included in the same momentum it was
submitted in (momentum_diff = 0 for all 13 transactions). This is remarkable —
even with 10s momenta, a tx submitted at momentum N is included in momentum N.

**Explanation:** The PoW computation happens *before* submission. By the time
the tx reaches the mempool, the next momentum hasn't been produced yet. With
10s momenta, there's approximately 10 seconds of mempool dwell time, which is
sufficient for the next producer to pick up the tx.

### 3. PoW Computation Time: Unchanged from Phase 9

| Metric | 1-producer (30s) | 3-producer (10s) |
|--------|------------------|------------------|
| PoW avg | 24,500-38,900ms | 24,397ms |
| PoW min | 3,278ms | 1,654ms |
| PoW max | 75,806ms | 59,734ms |

PoW computation time is a function of difficulty and CPU speed, not network
configuration. The multi-producer devnet does not affect PoW time.

### 4. Inclusion Latency: Near-Zero (0-3ms)

| Metric | 1-producer (30s) | 3-producer (10s) |
|--------|------------------|------------------|
| p50 | 1ms | 0-1ms |
| p90 | 18-22ms | 1-3ms |

Inclusion latency is actually slightly *better* with 3 producers, likely because
the next momentum is produced sooner (average 5s wait vs 15s), reducing the
time between submission and inclusion.

### 5. Mixed Contention: Fused Txs Still Dominate

From the mixcontend experiment (3 fused + 3 unfused, 2 txs each):

| Group | Accounts | Txs | PoW Time | Submit Momentum |
|-------|----------|-----|----------|-----------------|
| Fused | 100-102 | 6 | 0ms (plasma) | All at 10822 |
| Unfused | 2028 | 2 | 958ms, 1679ms | Both at 10822 |
| Unfused | 2027 | 2 | 11,746ms, 34,387ms | 10823, 10826 |
| Unfused | 2026 | 2 | 13,879ms, 37,652ms | 10823, 10827 |

**Pattern:** Fused txs are all submitted in the first momentum (0 PoW delay).
Unfused txs with fast PoW (<2s) also make the first momentum. Unfused txs with
slower PoW (11-38s) are spread across 2-4 momenta.

**100% inclusion for both groups** (12/12 total). No rejections.

### 6. Comparison with Phase 11 (1-producer, 30s momenta)

Phase 11 finding: fused txs get 100% first-momentum inclusion; unfused txs get
24-72% first-momentum inclusion (rest spread across 2-5 momenta).

Phase 24 finding: same pattern holds with 10s momenta. The absolute number of
momenta missed is similar, but each momentum is 3× shorter, so the absolute
time delay is 3× less.

## Implications

1. **PoW transactions are viable on multi-producer networks.** The 10s
   momentum rate does not disadvantage PoW txs compared to 30s momenta.
   Inclusion is still 100% and same-momentum for single txs.

2. **The mempool is the great equalizer.** Regardless of momentum rate,
   the mempool holds txs until the next momentum. PoW txs are never dropped
   just because they missed a momentum.

3. **Fused plasma still dominates for latency-sensitive applications.**
   While PoW txs achieve same-momentum inclusion, the PoW computation
   itself (17-60s) is the real latency bottleneck. Fused txs are included
   in 0-1ms with no computation delay.

4. **Rapid-fire self-transfers can hit temporary insufficient balance.**
   When an account sends multiple self-transfers rapidly, the balance drops
   temporarily (sends deduct immediately, receives add back later). This
   caused test failures with 3+ rapid txs from accounts with limited ZNN.
   This is a wallet/client consideration, not a protocol issue.

5. **Dynamic Plasma pricing unchanged.** No repricing was observed during
   PoW bursts (workPrice stayed at 1000), consistent with phase 9's finding
   that workPrice repricing is impractical to trigger via PoW alone.

## Raw Results

- `powlat-5acct-3p-seed0.json` — 5 accounts, seed 0 (all 5 included, same momentum)
- `powlat-5acct-3p-seed1.json` — 5 accounts, seed 1 (all 5 included, same momentum)
- `powlat-3acct-3p-seed2.json` — 3 accounts, seed 2 (all 3 included, same momentum)
- `mixcontend-3f-3u-2tx-3p-seed0-fix.json` — mixed contention, 3f+3u, 2 txs each
- `mixcontend-3f-3u-3tx-3p-seed0.json` — earlier attempt (test design issue: insufficient balance)

## Suite Changes

- `labctl powlat` now accepts optional baseIdx parameter (default 200)
- `labctl mixcontend` now accepts optional fusedBaseIdx, unfusedBaseIdx, and skipfusion flag
- `run-suite.sh` has new phase24 target

## Caveats

- Single-machine deployment: all 3 nodes share CPU. PoW times may be affected
  by CPU contention from the 3 znnd instances.
- The mixcontend experiment used 2 txs per account (not 3+) to avoid temporary
  insufficient balance from rapid-fire self-transfers.
- Small sample size (n=13 for powlat, n=12 for mixcontend). Larger samples would
  tighten confidence intervals.
- The mixcontend momentum contents analysis had a display bug (account indices
  not mapped correctly in the breakdown). Raw results are correct.
