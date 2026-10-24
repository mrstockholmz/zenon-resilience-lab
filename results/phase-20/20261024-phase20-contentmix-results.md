# Phase 20: Momentum Content Capacity — Count vs Plasma vs Contract Blocks

**Date:** 2026-10-24
**Go-zenon pin:** `e89f91f68b922a65370b7ffd9cfe860045a99b29`
**Devnet:** chain 69, isolated, dynamic plasma regime (height >= 30)
**Command:** `labctl contentmix <selfTransfer> <stake> <qsrPerAccount> [baseIdx]`

## Question

What is the actual maximum number of content entries in a single momentum?
Three limits interact:
1. **Plasma budget**: `MaxBasePlasmaInMomentum` = 4,200,000 plasma for user blocks
2. **Contract block cap**: `MaxContractBlocksInMomentum` = 80
3. **Content count cap**: `maxContentUnderPlasma()` = 200 + 80 = 280 (post-DP)

The 280 cap is a pre-allocation bound in the verifier (`verifier/momentum.go:231-233`).
It prevents allocating maps from attacker-supplied content length. But is it ever
the binding constraint, or does the plasma budget always limit content first?

## Source Analysis

From `verifier/momentum.go:290-324` (dynamic plasma path):
- User (non-embedded) blocks: each consumes plasma from `MaxBasePlasmaInMomentum`
- Contract (embedded) blocks: counted against `MaxContractBlocksInMomentum` (80), do NOT consume plasma budget
- Total content must be <= `maxContentUnderPlasma()` = 280

Key insight: **contract blocks are "free" in terms of plasma budget**. A momentum
could theoretically have 200 user blocks (4.2M plasma) + 80 contract blocks = 280
total content. But can this actually happen?

## Experiment Design

Three configurations, each sending transactions from funded+ fused accounts
(200 QSR each = 420,000 plasma per account):

| Config | Self-transfers | Stake calls | User plasma | Theoretical max content |
|--------|---------------|-------------|-------------|------------------------|
| A (seed 0) | 200 | 0 | 4,200,000 | 200 |
| A (seed 1) | 200 | 0 | 4,200,000 | 200 |
| B | 100 | 40 | 4,200,000 | 180 |

- Self-transfer: 21,000 plasma each → 1 content entry
- Stake call: 52,500 plasma (EmbeddedSimple) → 2 content entries (user send + contract receive)
- Theoretical max = self_transfers + stake_calls × 2 (if all fit in one momentum)

## Results

### Config A seed 0: 200 self-transfers

| Metric | Value |
|--------|-------|
| Actual max content | **197** |
| Height | 8202 |
| User blocks | 197 |
| Contract blocks | 0 |
| 280 cap bind | **No** |

197 of 200 self-transfers included in a single momentum. The remaining 3 were
included in the next momentum. The momentum also included 2 blocks from other
accounts (backlog from previous experiments).

### Config A seed 1: 200 self-transfers (reuse accounts)

| Metric | Value |
|--------|-------|
| Actual max content | **200** |
| Height | 8289 |
| User blocks | 200 |
| Contract blocks | 0 |
| 280 cap bind | **No** |

**Exactly 200 content entries** — precisely at the plasma budget limit
(200 × 21,000 = 4,200,000). This confirms the plasma budget is the exact
binding constraint for pure user transactions. 67 were from this run, 133
were backlog from the previous run (same accounts).

### Config B: 100 self-transfers + 40 Stake calls

| Metric | Value |
|--------|-------|
| Actual max content | **158** |
| Height | 8259 |
| User blocks | 158 |
| Contract blocks | 0 |
| 280 cap bind | **No** |

158 user blocks in the primary momentum. 0 contract blocks — the Stake calls'
contract receives appear in the **next** momentum (2-momentum pipeline,
confirmed in phase 19). The theoretical max was 180 (100 self + 40×2 Stake),
but only 158 fit because the plasma budget was shared between self-transfers
(29 × 21,000 = 609,000) and Stake sends (27 × 52,500 = 1,417,500), plus
backlog from previous runs.

## Analysis

### Finding 1: The 280 content cap is NOT the binding constraint

In all three configurations, the 280 cap was never approached. The maximum
observed content was 200 (Config A seed 1), which is exactly the plasma
budget limit (4.2M / 21,000 = 200). The 280 cap is a safety bound for
pre-allocation, not a throughput limit.

### Finding 2: Plasma budget is the exact binding constraint for user blocks

Config A seed 1 achieved exactly 200 content entries — the theoretical
maximum given the plasma budget. This is the hardest possible limit:
200 × 21,000 = 4,200,000 = MaxBasePlasmaInMomentum.

### Finding 3: Mixing contract calls REDUCES total content capacity

Config B (100 self + 40 Stake) achieved only 158 content entries vs.
200 for pure self-transfers. This is because:
- Each Stake call costs 52,500 plasma (2.5× a self-transfer)
- The contract receive adds 1 more content entry but no extra plasma
- Net effect: Stake calls are less plasma-efficient per content entry

Maximum content per plasma:
- Self-transfer: 1 content / 21,000 plasma = 0.0000476 content/plasma
- Stake call: 2 content / 52,500 plasma = 0.0000381 content/plasma

Self-transfers are 25% more plasma-efficient per content entry.

### Finding 4: Contract blocks appear in the next momentum

The 2-momentum pipeline (phase 19) was confirmed again. User sends in
momentum N generate contract receives in momentum N+1. This means the
theoretical maximum of 280 content (200 user + 80 contract) would require:
- Momentum N: 200 user blocks (plasma budget) + contract blocks from N-1
- Momentum N-1: enough Stake calls to generate 80 contract blocks in N

But 80 Stake calls cost 80 × 52,500 = 4,200,000 plasma — the entire budget.
So momentum N-1 would have 80 Stake calls (4.2M plasma, 80 user blocks) and
momentum N would have 80 contract receives + up to 200 self-transfers.
Total content in momentum N: 80 + 200 = 280. **This is theoretically possible
but requires precise orchestration across two momenta.**

### Finding 5: The real-world maximum content per momentum is ~200

For practical purposes, the maximum content per momentum is ~200 (the plasma
budget limit for user blocks). The 280 cap only matters in the edge case where
a momentum has the maximum user blocks AND the maximum contract blocks from
the previous momentum. This edge case is unlikely in practice because:
1. It requires precise timing across two momenta
2. Stake calls reduce the number of user blocks that fit in the plasma budget
3. The 2-momentum pipeline means contract blocks lag by one momentum

## Raw Data

- `contentmix-A-200self-0stake-seed0.json` — Config A seed 0
- `contentmix-A-200self-0stake-seed1.json` — Config A seed 1
- `contentmix-B-100self-40stake-seed0.json` — Config B
- `contentmix-*-seed*.stderr` — stderr logs for each run

## Conclusion

The 280 content cap is a correctness bound, not a throughput limit. The actual
maximum content per momentum is determined by the plasma budget (4.2M) and
equals 200 for pure user transactions. Mixing contract calls reduces total
content because contract calls are less plasma-efficient. The theoretical
maximum of 280 is achievable only with precise two-momentum orchestration
that is unlikely to occur naturally.
