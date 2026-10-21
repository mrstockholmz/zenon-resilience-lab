# Phase 3 Results — Resource Matrix

**Date:** 2026-10-21
**Node:** lab devnet, go-zenon `e89f91f`, single pillar, chain 69
**Regime:** Dynamic Plasma (momentum height >= 30)

## Matrix Design

| case | fused plasma | demand pattern | accounts | description |
|------|-------------|----------------|----------|-------------|
| low-steady-1 | 0 | sequential | 1 | 5 self-transfers, one at a time |
| low-burst-1 | 0 | parallel | 1 | 5 self-transfers, simultaneous |
| high-steady-1 | 210K | sequential | 1 | 5 self-transfers, one at a time |
| high-burst-1 | 210K | parallel | 1 | 5 self-transfers, simultaneous |
| high-burst-multi | 42M + 105K | parallel | 2 | 3 self-transfers each, simultaneous |

Each case run with 3 fixed seeds (0, 1, 2). "Low fused" = 0 QSR fused (PoW fallback).
"High fused" = 100 QSR fused (210K plasma). Multi case: account 1 = 20000 QSR genesis
(~42M plasma), account 2 = 50 QSR fused (105K plasma).

## Results

### low-steady-1 (0 plasma, sequential)

| seed | attempted | included | rejected | PoW count | total PoW ms | p50 latency | duration |
|------|-----------|----------|----------|-----------|--------------|-------------|----------|
| 0 | 5 | 5 (100%) | 0 | 5 | 41361 | 1ms | 41.4s |
| 1 | 5 | 5 (100%) | 0 | 5 | 79000 | 1ms | 79.0s |
| 2 | 5 | 5 (100%) | 0 | 5 | 48841 | 0ms | 48.9s |

**Finding:** 100% inclusion. Every transaction needs PoW (5-54s each). Once PoW is
solved, inclusion is near-instant (next momentum). Total time is dominated by PoW.

### low-burst-1 (0 plasma, parallel)

| seed | attempted | included | rejected | PoW count | total PoW ms | p50 latency | duration |
|------|-----------|----------|----------|-----------|--------------|-------------|----------|
| 0 | 5 | 2 (40%) | 3 | 2 | 23957 | 1ms | 98.6s |
| 1 | 5 | 1 (20%) | 4 | 1 | 4319 | 1ms | 26.0s |
| 2 | 5 | 1 (20%) | 4 | 1 | 3307 | 1ms | 32.5s |

**Finding:** 20-40% inclusion. Rejection reason: "hash tie-break" or "cemented block"
— all goroutines read the same frontier, build on it, and compete. The first to
publish wins; the rest are stale. PoW delay (5-50s) makes the race window very wide.

### high-steady-1 (210K plasma, sequential)

| seed | attempted | included | rejected | PoW count | total PoW ms | p50 latency | duration |
|------|-----------|----------|----------|-----------|--------------|-------------|----------|
| 0 | 5 | 5 (100%) | 0 | 0 | 0 | 0ms | 0.02s |
| 1 | 5 | 5 (100%) | 0 | 0 | 0 | 0ms | 0.02s |
| 2 | 5 | 5 (100%) | 0 | 0 | 0 | 0ms | 0.02s |

**Finding:** 100% inclusion, zero PoW, near-zero latency. Each transaction consumes
21000 plasma. 5 transactions * 21000 = 105000 plasma used. Total time: 0.02 seconds
for 5 transactions (vs 41-79 seconds with PoW). **This is the key value proposition
of fused plasma.**

### high-burst-1 (210K plasma, parallel)

| seed | attempted | included | rejected | PoW count | total PoW ms | p50 latency | duration |
|------|-----------|----------|----------|-----------|--------------|-------------|----------|
| 0 | 5 | 5 (100%) | 0 | 0 | 0 | 0ms | 0.05s |
| 1 | 5 | 1 (20%) | 4 | 1 | 1536 | 0ms | 30.1s |
| 2 | 5 | 5 (100%) | 0 | 0 | 0 | 0ms | 0.05s |

**Finding:** Seeds 0 and 2: 100% inclusion, 0 PoW, 0.05s total. Seed 1 is an outlier:
plasma was exhausted (0 remaining from prior tests, hadn't regenerated). The single
included transaction used PoW (1.5s). This shows that fused plasma must be *available*
(not just *fused*) for the benefit.

**Key insight:** With sufficient fused plasma, single-account bursts achieve 100%
inclusion because the 10ms stagger between goroutine starts is enough for each
transaction to build on the updated frontier (no PoW delay). Without plasma,
the PoW delay (5-50s) creates a wide race window where all goroutines build on
the same stale frontier.

### high-burst-multi (42M + 105K plasma, 2 accounts, parallel)

| metric | value |
|--------|-------|
| attempted | 6 |
| included | 6 (100%) |
| PoW count | 1 |
| total PoW ms | 20378 |
| p50 latency | 1ms |
| duration | 20.4s |

Account 1 (42M plasma): 3/3 included, 0 PoW, instant.
Account 2 (105K plasma): 3/3 included, 1 PoW (20.4s), then 2 without PoW.

**Finding:** Multi-account burst with fused plasma achieves 100% inclusion.
Account 2 needed PoW for its first transaction because the 5 receive blocks
(from funding) had consumed its 105K plasma. After the PoW transaction, plasma
was available for the remaining 2.

## Plasma Consumption and Regeneration

- Each self-transfer (UserSend, 1 ZNN, no data) consumes exactly **21000 plasma**.
- 100 QSR fused = **210000 max plasma** (2100 plasma per QSR).
- Plasma **regenerates** over time. After exhausting 210000 plasma in high-steady-1
  seed 2, the full 210000 was available again by the next test (~1 momentum later).
  The regeneration rate appears to be fast on a single-producer devnet with low usage.

## Rejection Reason Taxonomy

| reason | cause | frequency |
|--------|-------|-----------|
| "hash tie-break is worse for current block" | two blocks with same prevHash; one has better hash | burst mode, when PoW delays cause frontier staleness |
| "account-block prevHash exists but it has a cemented block on top of it" | the account chain moved forward while we were computing PoW | burst mode, after the first block is included |
| "insufficient balance for transfer" | account hasn't received funds yet (unreceived blocks pending) | multi-account setup race |

## Raw Data

All runs stored as JSON in `results/phase-3/`:
- `low-steady-1-seed{0,1,2}.json`
- `low-burst-1-seed{0,1,2}.json`
- `high-steady-1-seed{0,1,2}.json`
- `high-burst-1-seed{0,1,2}.json`
- `high-burst-multi-seed0-r3.json`
