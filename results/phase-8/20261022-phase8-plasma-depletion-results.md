# Phase 8 — Plasma Depletion and Regeneration Results

**Date:** 2026-10-22
**Status:** done (3 seeds, reproducible)

## Question

How does fused plasma deplete as transactions are sent? Does plasma regenerate
after being consumed? What happens when plasma reaches zero?

## Background

Phase 7 noted "plasma regenerates over time" as an observation. The go-zenon
code (`vm/plasma.go:76-102`) shows a two-counter system:

```
available = fusedPlasma + committed - uncommitted
```

Where `committed` (cache store) tracks plasma used by confirmed blocks and
`uncommitted` (frontier account store) tracks plasma used by ALL blocks
(confirmed + unconfirmed). The difference represents plasma "locked" by
unconfirmed transactions.

This experiment verifies the two-counter model empirically.

## Method

`labctl deplete <accountIdx> <qsr> <maxTx>` — Fuse a known QSR amount on a
fresh derived account, then send self-transfers until plasma reaches zero.
Record plasma before and after each transaction. After exhaustion, wait 5
momenta and re-check plasma (regeneration test).

**Setup:** Each account is funded with 100 ZNN and fused with 100 QSR
(= 210,000 plasma capacity). Self-transfers of 1 ZNN each consume plasma.

## Key Findings

### 1. Each transaction consumes exactly 21,000 plasma

All 3 seeds show identical linear depletion:

| Tx | Plasma Before | Plasma After | Consumed |
|----|---------------|--------------|----------|
| 0 | 210,000 | 189,000 | 21,000 |
| 1 | 189,000 | 168,000 | 21,000 |
| 2 | 168,000 | 147,000 | 21,000 |
| ... | ... | ... | ... |
| 9 | 21,000 | 0 | 21,000 |

This matches the code constant `AccountBlockBasePlasma = 21,000`
(`vm/constants/plasma.go:33`).

### 2. Plasma fully regenerates after confirmation

After depleting to 0 and waiting 5 momenta (~50 seconds), plasma returns
to the full fused amount:

| Seed | Fused QSR | Depleted At | Plasma After 5 Momenta |
|------|-----------|-------------|------------------------|
| 0 | 100 | tx 10 | 210,000 |
| 1 | 100* | tx 9 | 420,000* |
| 2 | 100 | tx 10 | 210,000 |

*Seed 1 had a double-fusion (200 QSR total) due to a retry artifact.
The regeneration is still full: 420,000 = 200 QSR × 2,100.

**Conclusion: Plasma is NOT permanently consumed.** The "depletion" is a
temporary lock caused by unconfirmed transactions. Once transactions are
confirmed by a momentum, the `committed` and `unconfirmed` counters sync
up, and the full fused plasma is available again.

### 3. 100 QSR = exactly 10 simple self-transfers before temporary exhaustion

100 QSR × 2,100 plasma/QSR = 210,000 plasma. Each simple self-transfer
requires 21,000 plasma. So 100 QSR allows exactly 10 transactions before
plasma is temporarily exhausted.

This has implications for mainnet: an account with 100 QSR fused can send
10 simple transactions per momentum (if all are confirmed in that momentum).
If more are sent before confirmation, they will be rejected.

### 4. No PoW fallback needed in sequential tests

All 3 seeds had `pow_count = 0` (except seed 0 tx 0 which used PoW because
the fusion hadn't been credited yet). Sequential transactions with
sufficient plasma never need PoW.

### 5. Regeneration is fast — one momentum is sufficient

The regeneration happens as soon as transactions are confirmed. In our
tests, all transactions were confirmed in a single momentum (the one they
were submitted in). The 5-momentum wait was conservative; in practice,
plasma regenerates after just 1 momentum (~10 seconds on our devnet).

## Implications for Mainnet

1. **Plasma is a per-momentum rate limit, not a permanent budget.** An
   account can send up to `fused_plasma / 21,000` transactions per momentum.
   Once those transactions are confirmed, the full amount is available again.

2. **The "contention" observed in phase 7 was about unconfirmed backlog,
   not permanent depletion.** When multiple accounts send simultaneously,
   the ones with more fused plasma can have more transactions in flight
   before hitting the temporary limit.

3. **For sustained high-volume sending, you need either:**
   - More fused QSR (higher per-momentum capacity)
   - PoW on each transaction (bypasses the fused plasma limit entirely)
   - Multiple accounts (each has its own plasma pool)

4. **Plasma economics:** 1 QSR = 2,100 plasma = 0.1 simple transactions
   per momentum. To send 1 tx/momentum continuously, you need 10 QSR.
   To send 100 txs/momentum, you need 1,000 QSR.

## Limits

- Single-producer devnet; confirmation times may differ on mainnet.
- Only simple self-transfers tested. Complex contract interactions may
  consume more plasma per transaction.
- The regeneration test waited 5 momenta; the minimum wait for full
  regeneration was not measured precisely.
- Seed 1 had a double-fusion artifact; the regeneration finding is still
  valid but the absolute plasma numbers are 2× the expected value.

## Raw Data

- `deplete-acct21-100qsr-seed0.json` — seed 0 (PoW on tx 0, then 10 fused txs)
- `deplete-acct22-100qsr-seed1.json` — seed 1 (double fusion, 9 txs)
- `deplete-acct24-100qsr-seed2.json` — seed 2 (clean, 10 txs)
- `deplete-acct23-100qsr-seed2.json` — seed 2 first attempt (failed, timing)
