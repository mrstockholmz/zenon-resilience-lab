# Phase 6 — Dynamic Plasma Repricing: DONE

**Date:** 2026-10-22
**Status:** done (was proposed, now executed)
**Results:** results/phase-6/20261022-phase6-repricing-results.md

## What happened

The high-volume burst experiment was run locally on the lab devnet.
Three seeds of 200-transaction bursts all confirmed the repricing mechanism:

- >50 fused txs/momentum triggers fusion price increase
- Maximum increase is +10% per momentum (capped)
- Price decays ~5% per empty momentum to floor of 1000
- Sustained repricing requires sustained transaction load

## Decision informed

**When to activate dynamic-plasma spork on mainnet:** The repricing mechanism
is functional and well-behaved. Activation should not disrupt existing users
because (a) the 50-tx/momentum threshold is high, (b) the +10% cap limits
rate of change, and (c) prices decay quickly when load subsides.

## Feedback request (for post.py, not yet posted)

The phase 2–6 results are ready for maintainer review. The key question for
go-zenon maintainers: is the observed repricing behaviour (threshold, cap,
decay rate) consistent with the Dynamic Plasma design intent? A short review
of the phase 6 repricing report would confirm the lab's methodology before
the next experiment (multi-sender contention or PoW repricing).
