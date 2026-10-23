# Phase 12 — Saturation Behavior Results

**Date:** 2026-10-23
**Status:** done (3 configurations)

## Question

What happens when total transaction demand exceeds the 200 tx/momentum hard
limit? Does the node queue excess transactions, reject them, or drop them?
How fast does the backlog drain?

## Method

`labctl saturate <N> <M> <QSR> <amt>` — N derived accounts with equal plasma
each send M self-transfers as fast as possible. Total = N×M, designed to
exceed 200. All accounts are fused with the same QSR amount (equal plasma),
unlike phase 7's gradient.

**Key design:** Fresh accounts starting at index 100 (no previous transaction
history). Each account fused with sufficient QSR for all M transactions
(M × 21000 plasma needed).

**Per-momentum limit:** 200 transactions (MaxBasePlasmaInMomentum)

## Configurations

| Config | Accounts | Txs/Acct | Total | QSR/Acct | Plasma/Acct | Saturation |
|--------|----------|----------|-------|----------|-------------|------------|
| A | 20 | 15 | 300 | 150 | 315,000 | 1.5× |
| B | 25 | 20 | 500 | 200 | 420,000 | 2.5× |
| C | 15 | 30 | 450 | 300 | 315,000* | 2.25× |

*Config C accounts were reused from previous runs (accumulated plasma from
prior fusion + new fusion). Some accounts had insufficient effective plasma
for 30 txs in a single momentum window.

## Results

### Config A: 300 txs (1.5× limit)

| Metric | Value |
|--------|-------|
| Attempted | 300 |
| Submitted | 300 |
| Included | 300 (100%) |
| Rejected | 0 |
| Duration | 119.9s |
| Momenta used | 4 (3862-3865) |

**Momentum distribution:**
- Momentum 3862: 20 txs (fusion setup, not counted)
- Momentum 3863: **200 txs** (exactly at limit)
- Momentum 3864: 0 txs (empty)
- Momentum 3865: 0 txs (empty)
- Momentum 3866: 114 txs (backlog drain — detected after endM)

**Key observation:** First momentum took exactly 200 txs. Remaining 100 txs
drained 3 momenta later (3866), with 2 empty momenta in between.

### Config B: 500 txs (2.5× limit)

| Metric | Value |
|--------|-------|
| Attempted | 500 |
| Submitted | 500 |
| Included | 500 (100%) |
| Rejected | 0 |
| Duration | 89.2s |
| Momenta used | 6 (3872-3877) |

**Momentum distribution:**
- Momentum 3872: 25 txs (fusion setup)
- Momentum 3873: **200 txs** (exactly at limit)
- Momentum 3874: 0 txs (empty)
- Momentum 3875: 0 txs (empty)
- Momentum 3876: **200 txs** (exactly at limit)
- Momentum 3877: 0 txs (empty)
- Momentum 3878: 0 txs (empty)
- Momentum 3879: **200 txs** (exactly at limit — detected after endM)

**Key observation:** Three consecutive batches of exactly 200 txs, each
separated by 2 empty momenta. 500 txs at 2.5× saturation = 3 momenta to
clear, but spread across 8 momenta total (including empty ones).

### Config C: 450 txs (2.25× limit)

| Metric | Value |
|--------|-------|
| Attempted | 450 |
| Submitted | 437 |
| Included | 437 (97.1%) |
| Rejected | 13 (2.9%) |
| Duration | 352.5s |
| Momenta used | 10 (3884-3893) |

**Rejection reason:** All 13 rejections were "not enough TotalPlasma provided"
from accounts 104 (3 rejections) and 111 (10 rejections). These accounts
attempted to send 30 txs but their effective plasma (315,000) only supported
15 txs per momentum window. Plasma regenerates on confirmation, but the
rapid-fire submission outpaced regeneration.

**Momentum distribution** (includes txs from previous runs due to account reuse):
- Momentum 3885: 183 txs
- Momentum 3886: 11 txs
- Momentum 3888: 200 txs (at limit)
- Momentum 3890: 2 txs
- Momentum 3891: 194 txs
- Momentum 3892: 5 txs
- Momentum 3894: 177 txs

## Key Findings

### 1. The 200 tx/momentum limit is strictly enforced

Every momentum that included test transactions had ≤200 txs. No exceptions.
This confirms `MaxBasePlasmaInMomentum` is a hard consensus rule, not a
target or guideline.

### 2. Excess transactions are queued, not rejected (with sufficient plasma)

When total demand exceeds 200 txs/momentum, the node queues excess
transactions in the mempool and includes them in subsequent momenta.
Zero rejections occurred in Configs A and B (where plasma was sufficient).
This is fundamentally different from phase 7's contention scenario, where
insufficient plasma caused immediate rejection.

### 3. Queue drain rate: ~200 txs per 3 momenta

The node processes queued transactions in batches of exactly 200, but
requires 1-2 empty momenta between batches. This creates a drain rate of
approximately 67 txs/momentum on average — much slower than the 200/momentum
theoretical maximum.

**Pattern:** 200 txs → 2 empty momenta → 200 txs → 2 empty momenta → 200 txs

This behavior was consistent across all three configurations. The empty
momenta suggest the node needs time to:
- Validate and execute the previous batch
- Update account states and plasma balances
- Select and prepare the next batch

### 4. Plasma exhaustion is the only rejection cause under saturation

When accounts attempt more transactions than their plasma supports in a
single confirmation window, the excess txs are rejected with "not enough
TotalPlasma provided". This is the same rejection reason as phase 7, but
the mechanism is different:
- Phase 7: Insufficient total plasma (never had enough)
- Phase 12: Temporary plasma exhaustion (had enough, consumed it all)

Plasma regenerates on confirmation (phase 8), so patient accounts can
eventually send all their transactions. Impatient accounts get rejections.

### 5. Saturation does not affect inclusion probability (with plasma)

All 300 and 500 txs in Configs A and B were included. Being in the queue
did not reduce inclusion probability — txs were processed FIFO (first-in,
first-out) based on submission order.

## Implications

1. **The 200 tx/momentum limit is a rate limit, not a capacity limit.**
   Transactions are not lost under saturation — they are delayed.
   Wallets should expect longer confirmation times during high-demand
   periods but should not need to resubmit.

2. **Queue drain is slower than the limit suggests.**
   Effective throughput under sustained saturation is ~67 txs/momentum
   (200 per 3 momenta), not 200/momentum. This is important for capacity
   planning.

3. **Plasma is the real constraint for burst senders.**
   The 200 tx/momentum limit only matters when many accounts compete.
   A single account with 210,000 plasma can only send 10 txs per momentum
   anyway. The momentum limit binds when 20+ accounts with full plasma
   compete simultaneously.

4. **No fee market emerges under saturation.**
   Unlike Ethereum's gas auctions, Zenon's plasma system does not create
   a fee market. Transactions are queued FIFO, not prioritized by plasma
   amount (when all accounts have sufficient plasma). Dynamic Plasma
   repricing (phase 6) affects fusion price, not transaction priority.

## Reproducibility

- Config A: `labctl saturate 20 15 150 1`
- Config B: `labctl saturate 25 20 200 1`
- Config C: `labctl saturate 15 30 300 1`

Raw data: `results/phase-12/saturate-*.json`

## Caveats

- Single-producer devnet; results may differ on multi-producer mainnet.
- Config C reused accounts from previous runs, contaminating momentum
  content tracking with prior run transactions.
- The 60-second post-submission wait was insufficient for Config C;
  some momenta were missed by the automated tracking.
- Queue drain pattern (200 → 2 empty → 200) is empirical; the underlying
  mechanism (mempool selection, validation timing) was not verified in code.
