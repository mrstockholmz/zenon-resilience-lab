# Phase 9 — PoW Latency and Cost Measurement Results

**Date:** 2026-10-22
**Status:** done (3 seeds, reproducible)

## Question

What is the real-world cost of using PoW (no fused plasma) for simple transactions
under Dynamic Plasma? How does PoW latency compare to fused plasma? Can workPrice
repricing be triggered through PoW alone?

## Background

Phases 2-8 established that fused plasma eliminates PoW entirely (0.02s vs 41-79s
for 5 txs). Phase 6 measured fusion price repricing (>50 fused txs/momentum triggers
+10% cap). This phase measures the PoW side:

- Actual PoW computation time on lab hardware (i7-4770, nice -n 10)
- End-to-end latency for unfused accounts
- Whether workPrice repricing is achievable through PoW alone

## Method

`labctl powlat <n> <amt>` — Uses N pre-funded unfused accounts (0 fused plasma).
Each account sends 1 self-transfer with PoW, all in parallel. Measures:
- PoW computation time per tx
- Inclusion latency (publish → confirmed)
- Required difficulty (from RPC)
- nextWorkPrice before/after

**Setup:** 14 accounts (indices 200-213), each with 200 ZNN balance, 0 QSR fused,
0 plasma. All send 1 ZNN self-transfers simultaneously.

**Hardware:** i7-4770 CPU, Go SHA3-256 benchmarked at ~1.8 MH/s under nice -n 10.

## Key Findings

### 1. PoW time has enormous variance: 3.3s to 75.8s per transaction

| Seed | Min PoW | Max PoW | Avg PoW | Total PoW |
|------|---------|---------|---------|-----------|
| 0 | 3,278ms | 45,829ms | 24,522ms | 343s |
| 1 | 3,603ms | 57,555ms | 26,030ms | 364s |
| 2 | 3,596ms | 75,806ms | 38,986ms | 546s |

The variance is inherent to PoW: the number of hash iterations until finding a
valid nonce follows a geometric distribution with mean = difficulty. At
difficulty 31.5M and 1.8 MH/s, expected time is ~17.5s. Actual times range
from 3.3s (6x lucky) to 75.8s (4.3x unlucky).

**Implication:** PoW transaction time is unpredictable. A wallet cannot give a
reliable ETA for PoW-based sends. This is a UX argument for fusion.

### 2. 100% inclusion rate for PoW transactions

All 42 transactions (14 per seed × 3 seeds) were included. No rejections.

This differs from phase 2/3 results where unfused burst had 20-40% inclusion.
The difference: in phase 9, each account sends only 1 tx (no account-chain
conflict). In phases 2-3, a single account sends many txs rapidly, causing
prevHash conflicts.

**Implication:** PoW txs are reliable when sent sequentially from an account.
The rejection risk comes from account-chain conflicts (multiple unconfirmed
txs from the same account), not from PoW itself.

### 3. Inclusion latency is near-instant: 0-34ms

| Seed | Min Latency | Max Latency | Avg Latency |
|------|-------------|-------------|-------------|
| 0 | 0ms | 34ms | 5ms |
| 1 | 0ms | 24ms | 6ms |
| 2 | 0ms | 18ms | 5ms |

Once PoW is solved and the tx is published, inclusion happens in the very next
momentum (10s on this devnet). The PoW computation dominates total latency.

**Implication:** The bottleneck for unfused accounts is PoW computation, not
chain inclusion. Faster PoW (better hardware, GPU) would directly reduce
transaction latency.

### 4. workPrice repricing is impractical to trigger via PoW on this hardware

14 parallel PoW transactions in a single momentum did not trigger workPrice
repricing (nextWorkPrice stayed at 1000). The PowPlasmaTarget is 1,050,000
(50 simple txs). With PoW taking 3-76s per tx and momentums every 10s, each
account can complete at most 1 tx per 1-8 momentums.

To trigger workPrice repricing, we would need >50 PoW txs included in a single
momentum. This requires either:
- ~50+ accounts all computing PoW simultaneously (impractical on one CPU)
- Much faster PoW hardware (GPU, ASIC)
- Pre-computed PoW nonces (not possible — nonce depends on block hash)

**Implication:** On mainnet, workPrice repricing via PoW alone is very unlikely
to fire organically. The primary repricing risk comes from fused plasma usage
(phase 6), not PoW. This means Dynamic Plasma's PoW pricing mechanism is
effectively dormant under normal conditions.

### 5. Required difficulty formula confirmed

All transactions used difficulty = 31,500,000, which matches:
```
requiredDifficulty = basePlasma * workPrice / PriceScaleFactor * PoWDifficultyPerPlasma
                   = 21,000 * 1000 / 1000 * 1500
                   = 31,500,000
```

This confirms the RPC endpoint `getRequiredPoWForAccountBlock` returns the
correct difficulty at workPrice=1000.

## Comparison: PoW vs Fusion

| Metric | PoW (unfused) | Fusion (100 QSR) |
|--------|---------------|-------------------|
| Per-tx time | 3.3-75.8s (avg ~25s) | 0.02s |
| Inclusion latency | 0-34ms | 1-2ms |
| Inclusion rate (sequential) | 100% | 100% |
| Inclusion rate (burst) | N/A (too slow) | 100% |
| Predictability | Low (geometric distribution) | High |
| Cost per tx | ~25s CPU time | 100 QSR upfront |

## Implications for Mainnet

1. **PoW is a viable but slow fallback.** For occasional transactions from an
   unfused account, PoW works reliably but takes 3-76 seconds. This is
   acceptable for low-frequency users but unsuitable for applications needing
   fast or predictable confirmation.

2. **Fusion is strongly preferred for any volume.** At 100 QSR (~$X at current
   prices), an account can send 10 txs/momentum with 0.02s latency. The break-even
   is roughly 4 PoW txs — if you send more than 4 transactions ever, fusion is
   faster overall.

3. **workPrice repricing is a non-event on mainnet.** With 97 pillars and
   typical transaction volumes, PoW usage will rarely exceed 50 txs/momentum.
   The fusion price repricing (phase 6) is the more relevant mechanism.

4. **Wallet UX should communicate PoW variance.** Users without fusion should
   see a range ("10-60 seconds") not a point estimate, because the geometric
   distribution of PoW time makes precise ETA impossible.

## Limits

- Single-producer devnet; mainnet may have different inclusion dynamics.
- Lab hardware (i7-4770, nice -n 10) is slower than typical user hardware.
  Modern CPUs might do PoW 2-5x faster.
- Only simple self-transfers tested. Complex contract interactions may require
  more plasma (higher difficulty).
- workPrice repricing was not triggered (14 txs < 50 target). The conclusion
  that it's "impractical" is based on hardware constraints, not a protocol
  limitation.

## Raw Data

- `powlat-14acct-seed0.json` — seed 0 (14 txs, all included)
- `powlat-14acct-seed1.json` — seed 1 (14 txs, all included)
- `powlat-14acct-seed2.json` — seed 2 (14 txs, all included)
- `powburst-20acct-60s-seed0.json` — initial powburst attempt (failed setup, superseded by powlat)
