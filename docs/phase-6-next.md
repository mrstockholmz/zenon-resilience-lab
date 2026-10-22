# Phase 6 — Next Experiment Proposal

**Date:** 2026-10-22
**Status:** proposed (not yet run)

## Decision the results inform

**Primary: When should the dynamic-plasma spork be activated on mainnet?**

The phase 4 finding — no observable difference between static and dynamic regimes for
fused accounts at low volume — directly informs this decision. Dynamic Plasma pricing
only fires when momentum-level plasma usage strictly exceeds the target. At the volumes
tested (5–10 simple transfers per block), the repricing never triggers. This means
activating the spork on mainnet should not disrupt existing fusion holders or
applications making ordinary transfers.

**Secondary: How much QSR should a wallet or dApp recommend for fusion?**

The 21000-plasma-per-transfer figure and the 100% inclusion rate for fused accounts
gives builders a concrete sizing formula: fusing N QSR provides N * 2100 max plasma,
which covers N * 0.1 simple transfers before regeneration.

## Recommended next experiment

**High-volume burst to trigger Dynamic Plasma repricing.**

All results so far used volumes well below the repricing threshold. The natural
next step is to push transaction volume high enough to trigger the dynamic pricing
mechanism and observe:

1. At what volume does repricing fire? (momentum-level plasma usage > target)
2. How much does the plasma cost increase for subsequent transactions?
3. Does the increased cost actually throttle the sender, or does the
   account-chain conflict limit dominate?
4. Does a lightly-provisioned probe account get priced out?

### Proposed design

- Fuse agent account with high plasma (e.g. 500+ QSR worth)
- Send parallel self-transfers at maximum rate (no stagger) for 60 seconds
- Record: plasma usage per momentum, repricing events, inclusion rate, latency
- Run probe account (low plasma) concurrently to observe competitive exclusion
- Compare against same workload in static regime (height < 30)

### Gate

PASS if we observe at least one repricing event and can report the before/after
plasma costs. FAIL if volume cannot reach the repricing threshold with a single
sender (may need multiple senders).

## Alternative: multi-sender contention

If a single sender cannot generate enough volume, run the same workload from
multiple funded accounts simultaneously. This tests whether contention between
senders (not just self-transfer conflicts) triggers repricing.

## Feedback request (for post.py, not yet posted)

The phase 2–4 results are ready for maintainer review. The key question for
go-zenon maintainers: is the observed 21000-plasma-per-transfer figure and the
no-difference-at-low-volume result consistent with the Dynamic Plasma design
intent? A short review of the phase 4 comparison report would confirm the lab's
methodology before the high-volume experiment begins.
