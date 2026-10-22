# Phase 6 — Dynamic Plasma Repricing Results

**Date:** 2026-10-22
**Status:** done (3 seeds, reproducible)

## Question

At what transaction volume does Dynamic Plasma repricing fire, and how much does
the plasma cost increase?

## Method

High-volume burst (`hvburst`) from the agent account (fused with 9500 QSR =
19,950,000 plasma). Each self-transfer consumes 21,000 plasma. The repricing
target is `FusedPlasmaTarget = 1,050,000` base plasma per momentum (50
transactions). The maximum per momentum is `MaxBasePlasmaInMomentum =
4,200,000` (200 transactions).

Three runs of 200 transactions each, sent as fast as possible (~1 second).
The momentum production rate on the devnet is ~10 seconds.

## Key Findings

### 1. Repricing threshold confirmed

>50 fused transactions per momentum triggers a fusion price increase.
The formula is:

```
change = currentPrice * (usedPlasma - targetPlasma) / targetPlasma / 20
```

At the default price of 1000 with 200 transactions (4,200,000 base plasma):
- change = 1000 * (4,200,000 - 1,050,000) / 1,050,000 / 20 = 150
- Capped at +10%: nextPrice = 1100

### 2. Repricing is capped at +10% per momentum

`MaxPriceChangePercent = 10` means the maximum increase per momentum is 10%,
even when usage is at maximum capacity (200 transactions = 4× target).

### 3. Price decays rapidly without sustained load

With 0 transactions in a momentum, the price drops by ~5% per momentum
(toward the floor of 1000 = `MinResourcePrice`):

| Momentum | Txs | nextFusionPrice |
|----------|-----|-----------------|
| N (full) | 200 | 1100 |
| N+1 (empty) | 0 | 1045 |
| N+2 (empty) | 0 | 1000 |

### 4. Three-seed reproducibility

All three runs show identical repricing behaviour:

| Seed | Txs in peak momentum | Peak nextFusionPrice |
|------|---------------------|---------------------|
| 0 | 158 | 1100 |
| 1 | 194 | 1100 |
| 2 | 200 | 1100 |

### 5. Sustained burst pattern

A 60-second sustain run (5 accounts, 3091 attempted, 1021 submitted)
produced an oscillating price pattern:

```
1100 → 1045 → 1000 → 1100 → 1045 → 1000
```

Each high-volume momentum (200 txs) triggers +10%, followed by decay over
2 empty momenta. The system never sustains elevated prices because the
transaction generation rate cannot keep up with the 10-second momentum
interval.

### 6. Account-chain bottleneck for single-account bursts

All transactions in a burst come from the same account, forming a sequential
chain. Only the winning chain gets included; orphaned blocks are discarded.
The `hvburst` tool reports "included" based on account height, which
overcounts — the actual number of included transactions matches the
momentum content count (e.g., 158 out of 200 attempted).

## Raw Data

- `hvburst-80-dynamic-seed0.json` — 80 txs (below max, 61 included)
- `hvburst-200-dynamic-seed0.json` — 200 txs, 158 included in momentum 976
- `hvburst-200-dynamic-seed1.json` — 200 txs, 194 included in momentum 999
- `hvburst-200-dynamic-seed2.json` — 200 txs, 200 included in momentum 1005
- `sustain-60s-5acct-dynamic-seed0.json` — 60s sustain, 5 accounts

## Implications for Mainnet

1. **Dynamic Plasma repricing is real and functional** on the pinned code.
2. **The +10% per-momentum cap means prices rise slowly** — even at 4×
   target usage, the maximum increase is 10% per 10-second momentum.
3. **Prices decay quickly** — a single empty momentum drops the price by
   ~5%, and two empty momenta return it to the floor.
4. **Sustained repricing requires sustained load** — sporadic bursts will
   not maintain elevated prices.
5. **The 50-transaction threshold per momentum is high** — on mainnet with
   multiple pillars and variable momentum times, reaching this threshold
   requires significant sustained demand.

## Limits

- Single-producer devnet; no network propagation effects.
- Account-chain conflicts reduce actual inclusion below attempted count.
- The momentum production rate (~10s) is slower than mainnet (~5-10s).
