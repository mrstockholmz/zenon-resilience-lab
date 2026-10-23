# Phase 14: Content Selector Price Priority

**Date:** 2026-10-23
**Status:** done
**Lab commit:** e89f91f (pinned)

## Question

Does the pillar content selector prioritize transactions by price-per-base-plasma
when the mempool exceeds the per-momentum capacity? Phase 13 found "no plasma-based
prioritization under saturation" — but that test used transactions with identical
price ratios (all at floor price 1000). This phase tests whether transactions with
**different** price ratios are prioritized.

## Hypothesis

The content selector (`pillar/content_selector.go`) sorts transactions by
`dp.HigherPrice()`, which compares `priceValue(a)/a.BasePlasma` vs
`priceValue(b)/b.BasePlasma`. If this sorting is effective, higher-priced
transactions should be included in earlier momenta when demand exceeds capacity.

## Method

**Experiment 1 (confounded):** 4 groups × 3 accounts × 20 txs = 240 txs.
Groups overpay by 1x, 2x, 4x, 8x. Result: confounded by plasma exhaustion —
low-multiplier accounts ran out of plasma before the content selector could
discriminate.

**Experiment 2 (clean):** 4 groups × 1 account × 100 txs = 400 txs.
Each account has enough plasma for all 100 txs at its overpay multiplier.
Total 400 txs exceeds the 200 tx/momentum limit.

- Account 900 (1x): FusedPlasma = 21,000 per tx, 2,520,000 plasma total
- Account 1000 (2x): FusedPlasma = 42,000 per tx, 5,040,000 plasma total
- Account 1100 (4x): FusedPlasma = 84,000 per tx, 10,080,000 plasma total
- Account 1200 (8x): FusedPlasma = 168,000 per tx, 20,160,000 plasma total

Price ratios (at floor prices 1000/1000):
- 1x: priceValue = 21,000 × 1000 = 21,000,000; ratio = 21,000,000 / 21,000 = 1000
- 2x: priceValue = 42,000 × 1000 = 42,000,000; ratio = 42,000,000 / 21,000 = 2000
- 4x: priceValue = 84,000 × 1000 = 84,000,000; ratio = 84,000,000 / 21,000 = 4000
- 8x: priceValue = 168,000 × 1000 = 168,000,000; ratio = 168,000,000 / 21,000 = 8000

## Results

### Experiment 2 (clean test)

| Group | Attempted | Included | Rejected | Unresolved |
|-------|-----------|----------|----------|------------|
| 1x    | 100       | 100      | 0        | 0          |
| 2x    | 100       | 100      | 0        | 0          |
| 4x    | 100       | 100      | 0        | 0          |
| 8x    | 100       | 100      | 0        | 0          |

**Momentum contents:**

| Momentum | 8x txs | 4x txs | 2x txs | 1x txs | Total |
|----------|--------|--------|--------|--------|-------|
| 4875     | 100    | 100    | 0      | 0      | 200   |
| 4876     | 0      | 0      | 100    | 0      | 100   |
| 4877+    | 0      | 0      | 0      | 100    | 100   |

**100% inclusion in all groups.** No rejections. The content selector deferred
lower-priced transactions to later momenta without dropping them.

## Key Findings

1. **Content selector price priority is CONFIRMED.** When transactions have
   different price-per-base-plasma ratios, the content selector includes
   higher-priced transactions first. This is a strict ordering, not a probabilistic
   one.

2. **Phase 13's conclusion is refined, not overturned.** Phase 13 tested
   transactions with identical price ratios (all at floor price 1000). In that
   case, the content selector's sort is a no-op, and the tie-break is by hash
   (effectively random). Phase 13 correctly observed no priority under those
   conditions. Phase 14 shows that priority **does** exist when prices differ.

3. **A fee market exists in principle.** A transaction can overpay
   (FusedPlasma > BasePlasma) to gain priority. The overpayment is a real cost:
   it consumes more plasma per transaction, reducing the total number of
   transactions an account can send per momentum.

4. **In practice, the fee market is dormant.** At floor prices, all transactions
   have the same price ratio, so there is no priority. To gain priority, a user
   must deliberately overpay, which requires fusing more QSR than the minimum.

## Code References

- `pillar/content_selector.go:118-161` — sortBlocksByPriority (k-way merge by price)
- `pillar/content_selector.go:326-335` — compareBlockPriority (HigherPrice + hash tie-break)
- `dp/dp.go:227-232` — priceValue (FusedPlasma × workPrice + PoWPlasma × fusionPrice)
- `dp/dp.go:234-248` — HigherPrice (price-per-base-plasma comparison)
- `vm/supervisor.go:405-410` — setBlockPlasma (auto-sets FusedPlasma = basePlasma when both Difficulty and FusedPlasma are 0)

## Raw Data

- `overpay-4g-3a-20tx-seed0.json` — Experiment 1 (confounded by plasma exhaustion)
- `overpay-4g-1a-100tx-seed0.json` — Experiment 2 (clean test)
- `overpay-4g-3a-20tx-seed0.stderr` — Experiment 1 stderr
- `overpay-4g-1a-100tx-seed0.stderr` — Experiment 2 stderr

## Limits

- Single-producer devnet tests resource mechanics only, not decentralization or
  consensus security.
- The experiment uses self-transfers (simplest transaction type). Contract
  interactions may have different plasma costs and priority behavior.
- The overpay multipliers (1x-8x) are extreme. In practice, smaller overpayments
  may not create meaningful priority differences.
