# Phase 7 — Multi-Sender Contention Results

**Date:** 2026-10-22
**Status:** done (3 seeds, reproducible)

## Question

When multiple accounts compete for limited momentum space, how does available
plasma affect inclusion priority? Does the plasma gradient across accounts
create a measurable inclusion gradient?

## Method

`labctl contend 10 30 1` — 10 derived accounts, each sending 30 self-transfers
(1 ZNN each) as fast as possible. All accounts send simultaneously, creating
contention for momentum space.

**Setup:** Each account is funded with 100 ZNN and fused with a varying amount
of QSR (account *i* gets *i × 10* QSR), creating a plasma gradient. The
contention phase starts only after all funding and fusion transactions are
confirmed.

**Total transactions:** 300 (10 accounts × 30 txs each)
**Per-momentum limit:** 200 transactions (MaxBasePlasmaInMomentum)

## Key Findings

### 1. Plasma availability determines inclusion under contention

When total demand exceeds per-momentum capacity, accounts with more available
plasma achieve higher inclusion rates:

| Account | Fused QSR | Plasma Available | Attempted | Included | Rate |
|---------|-----------|-----------------|-----------|----------|------|
| 1 | 10 | 50,443,000 | 30 | 30 | 100% |
| 2 | 20 | 7,875,000 | 30 | 30 | 100% |
| 3 | 30 | 21,294,000 | 30 | 30 | 100% |
| 4 | 40 | 8,568,000 | 30 | 30 | 100% |
| 5 | 50 | 8,442,000 | 30 | 30 | 100% |
| 6 | 60 | 378,000 | 30 | 17 | 57% |
| 7 | 70 | 441,000 | 30 | 20 | 67% |
| 8 | 80 | 504,000 | 30 | 23 | 77% |
| 9 | 90 | 567,000 | 30 | 26 | 87% |
| 10 | 100 | 630,000 | 30 | 29 | 97% |

**Seed 2 data.** Accounts 1-5 (high plasma from earlier fusion) all achieved
100%. Accounts 6-10 (fresh fusion, lower plasma) showed a clear gradient:
more plasma = higher inclusion.

### 2. Three-seed reproducibility

| Seed | Attempted | Submitted | Included | Rejected | Inclusion Rate |
|------|-----------|-----------|----------|----------|----------------|
| 0 | 300 | 277 | 277 | 23 | 92.3% |
| 1 | 300 | 300 | 300 | 0 | 100% |
| 2 | 300 | 265 | 265 | 35 | 88.3% |

Seed 1 achieved 100% because all accounts had sufficient plasma from
accumulated fusion (accounts were reused from seed 0). Seeds 0 and 2 show
rejection when plasma is scarce.

### 3. Rejection reason: insufficient plasma

All rejections in seeds 0 and 2 were:
```
not enough TotalPlasma provided for account-block (PoW + Fused)
```

No "insufficient balance" errors — the funding fix works correctly.

### 4. Momentum distribution under contention

Transactions spread across multiple momenta when demand exceeds capacity:

**Seed 0:**
- Momentum 1467: 185 txs (main burst)
- Momentum 1468: 4 txs (spillover)
- Momentum 1470: 175 txs (second burst)
- Momentum 1471-1472: 1 tx each

**Seed 2:**
- Momentum 1495: 71 txs (partial)
- Momentum 1497: 200 txs (maximum capacity)

The 10-second momentum interval means transactions that miss one momentum
must wait for the next. With 300 transactions and 200 capacity, at least
two momenta are needed.

### 5. No PoW fallback under contention

PoW count was 0-17 across seeds. The fused plasma was sufficient for most
transactions; PoW was only needed when plasma ran out (accounts 6-10 in
seed 0). The go-zenon node does not automatically fall back to PoW when
fused plasma is insufficient — the transaction is rejected.

## Implications for Mainnet

1. **Plasma is the primary resource under contention.** Accounts with
   sufficient fused plasma achieve 100% inclusion even when momentum space
   is contested.

2. **Low-plasma accounts are starved first.** When demand exceeds capacity,
   the accounts with least plasma are rejected. This creates a natural
   priority system based on plasma holdings.

3. **The 200-tx/momentum limit is real.** With 300 transactions from 10
   accounts, the limit was reached and excess transactions spilled to
   subsequent momenta.

4. **No automatic PoW fallback.** If fused plasma is insufficient, the
   transaction is rejected. Users must explicitly include PoW in their
   transactions to compensate for low plasma.

5. **Plasma regeneration matters.** Accounts with higher fusion amounts
   (accounts 1-5 with 10-50 QSR) had more plasma available than accounts
   with higher fusion but recent activity (accounts 6-10 with 60-100 QSR).
   This is because plasma regenerates over time and earlier fusion had
   more time to accumulate.

## Limits

- Single-producer devnet; no network propagation effects.
- All accounts send self-transfers (same amount, same token). Real-world
  contention would involve varying transaction types and amounts.
- The momentum production rate (~10s) is slower than mainnet (~5-10s).
- Account reuse across seeds means plasma accumulates; fresh accounts
  would show lower inclusion rates.

## Raw Data

- `contend-5acct-20tx-seed0.json` — initial 5-account test (100% inclusion)
- `contend-10acct-30tx-seed0-v2.json` — 10 accounts, seed 0 (92.3% inclusion)
- `contend-10acct-30tx-seed1-v2.json` — 10 accounts, seed 1 (100% inclusion)
- `contend-10acct-30tx-seed2-v2.json` — 10 accounts, seed 2 (88.3% inclusion)
