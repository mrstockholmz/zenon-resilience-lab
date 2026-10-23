# Phase 15: Plasma Regeneration Rate

**Date:** 2026-10-23
**Status:** done
**Lab commit:** e89f91f (pinned)

## Question

Phase 8 found that plasma regenerates after depletion but the rate was TBD.
This phase measures the actual regeneration rate: how quickly does an account's
available plasma return to the fused amount after full depletion?

## Hypothesis

The go-zenon code (vm/plasma.go) uses a two-counter system: `committed` and
`unconfirmed`. When a transaction is confirmed, the unconfirmed counter
decreases and the committed counter increases. Plasma available = fused -
committed - unconfirmed. If this model is correct, plasma should "regenerate"
(immediately return to full) as soon as the transactions are confirmed in a
momentum — not gradually over time.

## Method

Fuse a known QSR amount to a fresh account, deplete plasma by sending
self-transfers until exhausted, then sample plasma levels at increasing
momentum intervals (1, 3, 6, 10 momenta).

**Test matrix:**
| Seed | Account | QSR Fused | Fused Plasma | Txs to Deplete |
|------|---------|-----------|--------------|----------------|
| 0    | 41      | 100       | 210,000      | 10             |
| 1    | 42      | 100       | 210,000      | 10             |
| 2    | 43      | 50        | 105,000      | 5              |
| 3    | 44      | 10        | 21,000       | 1              |

All runs on the isolated lab devnet (chain 69, dynamic plasma regime,
height >> 30, prices at floor 1000/1000).

## Results

### Seed 0 (account 41, 100 QSR)

| Event | Plasma | Momentum |
|-------|--------|----------|
| Post-fusion | 210,000 | — |
| After depletion (10 txs) | 0 | 5290 |
| After 1 momentum | **210,000** | 5291 |
| After 3 momenta | 210,000 | 5293 |
| After 6 momenta | 210,000 | 5296 |
| After 10 momenta | 210,000 | 5300 |

### Seed 1 (account 42, 100 QSR)

| Event | Plasma | Momentum |
|-------|--------|----------|
| Post-fusion | 210,000 | — |
| After depletion (10 txs) | 0 | 5304 |
| After 1 momentum | **210,000** | 5305 |
| After 3 momenta | 210,000 | 5307 |
| After 6 momenta | 210,000 | 5310 |
| After 10 momenta | 210,000 | 5314 |

### Seed 2 (account 43, 50 QSR)

| Event | Plasma | Momentum |
|-------|--------|----------|
| Post-fusion | 105,000 | — |
| After depletion (5 txs) | 0 | 5316 |
| After 1 momentum | **105,000** | 5317 |
| After 3 momenta | 105,000 | 5319 |
| After 6 momenta | 105,000 | 5322 |
| After 10 momenta | 105,000 | 5326 |

### Seed 3 (account 44, 10 QSR)

| Event | Plasma | Momentum |
|-------|--------|----------|
| Post-fusion | 21,000 | — |
| After depletion (1 tx) | 0 | 5329 |
| After 1 momentum | **21,000** | 5330 |
| After 3 momenta | 21,000 | 5332 |
| After 6 momenta | 21,000 | 5335 |
| After 10 momenta | 21,000 | 5339 |

## Key Findings

1. **Plasma regenerates from 0 to full within a single momentum.** In all 4
   seeds, the plasma level returned to the full fused amount by the first
   momentum after depletion. There is no gradual regeneration curve — it is
   instantaneous on momentum confirmation.

2. **Plasma is NOT a rate limit in the traditional sense.** A more accurate
   description: plasma is a **per-momentum budget**. An account can spend its
   entire fused plasma in one momentum, and the full amount is available again
   in the very next momentum. There is no cooldown or regeneration period.

3. **The two-counter model is confirmed.** The go-zenon code's
   committed/unconfirmed system means "depletion" is just unconfirmed backlog.
   Once the momentum confirms, the unconfirmed counter drops to zero and the
   full plasma is available again. The "regeneration" is not a time-based
   process but an accounting reconciliation.

4. **This corrects the phase 8 description.** Phase 8 stated "plasma
   regenerates over time (rate TBD)." The correct description is: plasma
   regenerates **on momentum confirmation**, not over time. The rate is
   effectively infinite — one momentum.

5. **Practical implication for users and wallets:** An account with 100 QSR
   fused can send 10 transactions per momentum, indefinitely, with zero wait
   between momenta. There is no daily or hourly cap — only the per-momentum
   limit matters. This makes plasma much more user-friendly than a
   time-based rate limit.

## Code References

- `vm/plasma.go` — two-counter system (committed/unconfirmed)
- `vm/embedded/impl/plasma.go` — PlasmaContract implementation
- `dp/dp.go` — plasma calculation and pricing

## Raw Data

- `regen-acct41-100qsr-seed0.json` — Seed 0 (100 QSR, 10 txs to deplete)
- `regen-acct42-100qsr-seed1.json` — Seed 1 (100 QSR, 10 txs to deplete)
- `regen-acct43-50qsr-seed2.json` — Seed 2 (50 QSR, 5 txs to deplete)
- `regen-acct44-10qsr-seed3.json` — Seed 3 (10 QSR, 1 tx to deplete)
- `regen-acct41-100qsr-seed0.stderr` — Seed 0 stderr log
- `regen-acct42-100qsr-seed1.stderr` — Seed 1 stderr log
- `regen-acct43-50qsr-seed2.stderr` — Seed 2 stderr log
- `regen-acct44-10qsr-seed3.stderr` — Seed 3 stderr log

## Limits

- Single-producer devnet tests resource mechanics only, not decentralization
  or consensus security.
- All tests used self-transfers (simplest transaction type). Contract
  interactions may have different plasma costs.
- The dynamic plasma regime was at floor prices (1000/1000). Under repricing
  conditions (high demand), the effective per-momentum budget may differ.
- Regeneration was measured from full depletion (0 plasma). Partial depletion
  may behave differently if some transactions are still unconfirmed.
