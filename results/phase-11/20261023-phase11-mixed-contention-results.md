# Phase 11 — Mixed Fused/Unfused Contention Results

**Date:** 2026-10-23
**Status:** done (4 repetitions, reproducible)

## Question

When fused (plasma) and unfused (PoW) accounts compete for momentum space,
do fused transactions get priority? Are PoW transactions discriminated against
once submitted? Does the PoW computation delay put unfused accounts at a
structural disadvantage even when total demand is under the per-momentum limit?

## Method

`labctl mixcontend 5 5 5 1` — 5 fused accounts (20-100 QSR each, plasma
gradient) + 5 unfused accounts (0 QSR, PoW for every tx), each sending 5
self-transfers of 1 ZNN simultaneously.

**Total transactions:** 50 (25 fused + 25 unfused)
**Per-momentum limit:** 200 transactions (MaxBasePlasmaInMomentum)
**Demand vs capacity:** 50 << 200 (under limit — no rejection expected)

**Setup:** Fused accounts (indices 1-5) are funded with 100 ZNN and fused
with i×20 QSR. Unfused accounts (indices 300-304) are funded with 100 ZNN,
no fusion. All accounts send self-transfers concurrently.

## Key Findings

### 1. Fused transactions are included in the first momentum; unfused trickle in

All 25 fused transactions are included in the very first momentum after the
contention phase starts. Unfused transactions are distributed across 2-5
momenta as their PoW computations complete.

| Run | First Momentum | Fused in M1 | Unfused in M1 | Remaining Unfused |
|-----|---------------|-------------|---------------|-------------------|
| 1 | 3363 | 25/25 (100%) | 18/25 (72%) | 7 across 3366-3369 |
| 2 | 3408 | 25/25 (100%) | 6/25 (24%) | 19 across 3409-3412 |
| 3 | 3430 | 25/25 (100%) | 7/25 (28%) | 18 across 3431-3434 |
| 4 | 3452 | 25/25 (100%) | 9/25 (36%) | 16 across 3453-3457 |

**Fused accounts achieve first-momentum inclusion 100% of the time** because
there is no PoW delay — transactions are built and submitted in milliseconds.

**Unfused accounts achieve first-momentum inclusion only 24-72% of the time**,
limited by how many PoW computations finish before the momentum closes (~10s
interval). The variance is high because PoW time is geometrically distributed
(3-76s per tx).

### 2. 100% inclusion for both groups (no rejection)

| Run | Fused Included | Unfused Included | Total |
|-----|---------------|------------------|-------|
| 1 | 25/25 (100%) | 25/25 (100%) | 50/50 |
| 2 | 25/25 (100%) | 25/25 (100%) | 50/50 |
| 3 | 25/25 (100%) | 25/25 (100%) | 50/50 |
| 4 | 25/25 (100%) | 25/25 (100%) | 50/50 |

When total demand is under the per-momentum capacity limit, **both plasma and
PoW transactions are eventually included**. PoW transactions are NOT
discriminated against once they reach the node — they are included in the next
available momentum.

### 3. PoW computation is the bottleneck for unfused accounts

Per-tx PoW time (ms) across unfused accounts (average of 5 txs per account):

| Account | Run 1 | Run 2 | Run 3 | Run 4 | Overall Avg |
|---------|-------|-------|-------|-------|-------------|
| 300 | — | 22,958 | 14,159 | 15,833 | ~17,650 |
| 301 | 48,334 | 15,123 | 18,955 | 26,450 | ~27,216 |
| 302 | 25,505 | 14,556 | 11,081 | 21,238 | ~18,095 |
| 303 | 20,375 | 30,421 | 24,965 | 13,336 | ~22,274 |
| 304 | 10,946 | 12,455 | 12,832 | 33,985 | ~17,555 |

**Average PoW per tx: 11-34s** (geometric distribution, consistent with
phase 9 measurement of 3.3-75.8s).

Total contention phase duration: 155-200s (2.5-3.3 minutes) for 50 txs.
The fused accounts finish in <1s; the unfused accounts take 2.5-3 minutes.

### 4. No plasma = no priority advantage, but PoW delay = structural disadvantage

Unfused accounts have 0 plasma. When they submit a tx with PoW, the tx is
treated the same as a plasma tx for inclusion purposes. There is no
"plasma priority" that pushes PoW txs to later momenta — the only reason
unfused txs land in later momenta is that they arrive later (PoW delay).

### 5. Structural disadvantage for PoW-only users

Even under low demand (50 txs vs 200 limit), unfused accounts experience:
- **Higher latency**: 2-5 momenta (20-50s) vs 1 momentum (<10s) for fused
- **Unpredictable inclusion time**: depends on PoW speed, which varies 3-76s per tx
- **More momenta touched**: increases risk of interaction with other chain activity

This disadvantage is structural, not discriminatory — it comes from the PoW
computation delay, not from any node policy.

## Four-Run Comparison

| Metric | Run 1 | Run 2 | Run 3 | Run 4 | Spread |
|--------|-------|-------|-------|-------|--------|
| Total txs | 50 | 50 | 50 | 50 | 0 |
| Included | 50 | 50 | 50 | 50 | 0 |
| Rejected | 0 | 0 | 0 | 0 | 0 |
| Duration (s) | ~180 | 182 | 155 | 200 | 45s |
| Fused in M1 | 25 | 25 | 25 | 25 | 0 |
| Unfused in M1 | 18 | 6 | 7 | 9 | 12 |
| Total PoW (ms) | ~526K | 478K | 410K | 554K | 144K |
| Momenta used | 4 | 4 | 4 | 5 | 1 |

**Reproducibility:** 100% inclusion is perfectly reproducible. The number of
unfused txs in the first momentum varies widely (6-18) due to PoW timing
randomness. Total duration varies by ~22%.

## Implications for Mainnet

1. **Under low demand, PoW txs are not discriminated against.** If you can
   wait 3-76s for PoW, your tx will be included in the next momentum.

2. **Fused plasma provides a structural latency advantage.** Fused txs are
   included in the first momentum; PoW txs wait 1-4 momenta. For
   time-sensitive operations (DEX trading, auction bidding), fused plasma
   is effectively required.

3. **PoW variance makes unfunded accounts unreliable for burst activity.**
   With 5 txs per account, the total PoW time ranges 55-242s. An unfunded
   account cannot reliably send bursts — the txs arrive too slowly.

4. **The 200-tx/momentum limit was not tested in this phase.** This experiment
   used 50 txs total, well under the limit. Phase 7 tested the over-limit case
   with fused-only accounts. Mixed fused/unfused contention under over-limit
   demand remains untested.

## Limits

- Single-producer devnet; no network propagation effects.
- All accounts send self-transfers (same amount, same token).
- The momentum production rate (~10s) is slower than mainnet (~5-10s).
- Account reuse across runs means plasma accumulates; fresh accounts would
  show slightly different plasma levels.
- Over-limit contention (200+ txs) not tested with mixed groups.
- Runs 2-4 reused the same accounts (1-5, 300-304); run 1 was the first
  ever use of these accounts.

## Raw Data

- `mixcontend-5f-5u-5tx-seed0.json` — run 2 (first complete file on disk;
  run 1 output was captured from task notification only)
- `mixcontend-5f-5u-5tx-seed1.json` — run 3
- `mixcontend-5f-5u-5tx-seed2.json` — run 4
- Note: "seed" here means repetition index, not random seed — the mixcontend
  command uses fixed account indices (1-5 fused, 300-304 unfused).
