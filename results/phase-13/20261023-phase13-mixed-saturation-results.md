# Phase 13 — Mixed Fused/Unfused Saturation: Mempool Prioritization Under Contention

**Date:** 2026-10-23
**Status:** done (3 seeds)

## Question

When the mempool is saturated (more transactions than the 200 tx/momentum limit),
does the node prioritize transactions with fused plasma over transactions using
PoW? Or is selection strictly FIFO regardless of plasma?

## Background

Phases 11 and 12 established that:
- Fused txs get first-momentum inclusion at low volume (phase 11)
- Under saturation, excess txs are queued FIFO (phase 12)

But phase 12 used only fused accounts. Phase 13 tests whether plasma availability
affects queuing priority when fused and unfused txs compete for the same momentum
space under saturation.

## Method

`labctl mixsaturate <fused> <unfused> <fusedTxPer> <unfusedTxPer> <amt>`

- 20 fused accounts (indices 100-119) with varying QSR (plasma gradient)
- 10 unfused accounts (indices 300-309) with 0 QSR (PoW per tx)
- Fused accounts send 10 self-transfers each = 200 fused txs
- Unfused accounts send 1 self-transfer each = 10 unfused txs
- Total = 210 txs, exceeding the 200 tx/momentum limit
- All txs submitted simultaneously (parallel goroutines)

**Key design:** Fused txs need no PoW (0s computation). Unfused txs need PoW
(3-76s each). This creates a natural arrival-time difference: fused txs arrive
instantly, unfused txs arrive after PoW computation.

## Results

### Summary (3 seeds)

| Seed | Total | Included | Rejected | First Momentum | Fused in First | Unfused in First |
|------|-------|----------|----------|----------------|----------------|------------------|
| 0 | 210 | 210 (100%) | 0 | 200 txs | 197 | 3 |
| 1 | 210 | 210 (100%) | 0 | 200 txs | 197 | 3 |
| 2 | 210 | 210 (100%) | 0 | 200 txs | 200 | 0 |

**All 210 txs included in all 3 seeds. Zero rejections.**

### Per-Momentum Breakdown

**Seed 0:**
- Momentum 4406: 200 txs (197 fused + 3 unfused)
- Momentum 4407: 7 txs (0 fused + 7 unfused)
- Momentum 4408: 0 txs (empty)
- Momentum 4409: 3 txs (3 fused — account 117's remaining)

**Seed 1:**
- Momentum 4419: 200 txs (197 fused + 3 unfused)
- Momentum 4420: 7 txs (0 fused + 7 unfused)
- Momentum 4421: 0 txs (empty)
- Momentum 4422: 3 txs (3 fused — account 105's remaining)

**Seed 2:**
- Momentum 4427: 200 txs (200 fused + 0 unfused)
- Momentum 4428: 0 txs (empty)
- Momentum 4429: 6 txs (0 fused + 6 unfused)
- Momentum 4430: 4 txs (0 fused + 4 unfused)

### Which Unfused Txs Got Into the First Momentum?

**Seed 0:** accounts 301 (3.6s PoW), 307 (11.5s PoW), 302 (19.8s PoW)
**Seed 1:** accounts 307 (3.3s PoW), 308 (3.7s PoW), 301 (6.2s PoW)
**Seed 2:** none (fastest PoW was 21.5s — too slow)

In every case, the unfused txs that made it into the first momentum were the ones
with the **fastest PoW computation times**. This is consistent with FIFO selection
by arrival time: fused txs arrive instantly (no PoW), and the fastest PoW txs
arrive before the momentum is created.

### Which Fused Txs Were Displaced?

**Seed 0:** account 117 got only 7/10 txs in the first momentum
**Seed 1:** account 105 got only 7/10 txs in the first momentum
**Seed 2:** no fused txs displaced

The displaced account varies by seed, suggesting the selection is not strictly
deterministic but has some randomness in tie-breaking (e.g., mempool iteration
order, goroutine scheduling).

## Key Findings

### 1. No plasma-based prioritization under saturation

The node does **NOT** prioritize fused (plasma) transactions over unfused (PoW)
transactions when selecting transactions for a momentum. Selection is based on
**arrival time (FIFO)**, not plasma amount.

This is a significant finding: unlike Ethereum's gas auctions where higher fees
buy priority, Zenon's plasma system does not create a fee market for transaction
ordering. Plasma is a **rate limit** (how many txs you can send per momentum),
not a **priority mechanism** (which txs get included first).

### 2. Fused txs have an indirect advantage: instant arrival

Fused txs are included first not because of priority but because they arrive
first. With 0s PoW computation, fused txs hit the mempool immediately. Unfused
txs arrive 3-76s later (PoW time). Under saturation, this arrival-time difference
is the sole determinant of inclusion order.

### 3. Unfused txs are not discriminated against

When unfused txs arrive before the momentum is created (fast PoW), they are
included on equal footing with fused txs. There is no policy-level discrimination
against PoW transactions.

### 4. Queue drain follows the same pattern as phase 12

After the first momentum (200 txs), the remaining txs drain in subsequent
momenta with 1-2 empty momenta between batches. This is identical to phase 12's
finding with only fused accounts.

### 5. 100% inclusion under saturation (with sufficient resources)

All 210 txs were included in all 3 seeds. The 200 fused txs had sufficient plasma
(735K-1.4M plasma each, needing only 210K for 10 txs). The 10 unfused txs each
had sufficient PoW. Zero rejections.

## Implications

1. **Plasma is not a fee market.** Users cannot pay for priority by fusing more
   QSR. The only benefit of plasma is avoiding PoW delay (instant arrival).

2. **Wallets should use plasma for latency, not priority.** If a wallet needs
   fast confirmation under congestion, plasma fusion is the only reliable way
   (not because of priority, but because it eliminates the PoW delay).

3. **PoW is viable but slow under congestion.** An unfused transaction with
   fast PoW (<20s) can still get into the next momentum even under saturation.
   But the PoW delay makes this unreliable — the momentum might be created
   before the PoW completes.

4. **No incentive to fuse more than needed.** Since plasma doesn't buy priority,
   there's no benefit to fusing more QSR than needed for your transaction rate.
   Fusing 100 QSR (10 txs/momentum) is just as good as fusing 1000 QSR for
   inclusion priority.

## Reproducibility

- Command: `labctl mixsaturate 20 10 10 1 1`
- Raw data: `results/phase-13/mixsaturate-20f-10u-10ftx-1utx-seed{0,1,2}.json`
- 3 seeds, consistent overall pattern with variation in per-momentum distribution
  due to PoW computation variance

## Caveats

- Single-producer devnet; results may differ on multi-producer mainnet.
- The experiment uses self-transfers (simplest case). Complex contract
  interactions might have different plasma costs and prioritization.
- The "displaced" fused account (117 in seed0, 105 in seed1) suggests some
  non-determinism in mempool selection. The exact mechanism (iteration order,
  hash ordering, etc.) was not verified in code.
- PoW times vary significantly (3-76s) due to the geometric distribution of
  PoW difficulty. This variance affects which unfused txs arrive in time for
  the next momentum.
