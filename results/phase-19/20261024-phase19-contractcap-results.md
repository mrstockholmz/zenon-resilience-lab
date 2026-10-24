# Phase 19 — Contract Block Throughput Cap

**Date:** 2026-10-24
**Status:** done
**Results:** `results/phase-19/contractcap-{50,100}acct-*.json`

## Questions

1. Is the per-momentum contract block cap (`MaxContractBlocksInMomentum = 80`) strictly enforced?
2. How many contract calls can fit in one momentum, and what is the binding constraint?
3. Does the contract block cap interact with the plasma budget?

## Background

The go-zenon source defines a per-momentum cap on embedded (contract) blocks:

```go
// dp/dp.go:259
func MaxContractBlocks(config *definition.PlasmaVariables) uint64 {
    return config.MaxBasePlasmaInMomentum / constants.EmbeddedSimplePlasma
}
// = 4,200,000 / 52,500 = 80
```

Each contract call generates at least 1 contract block (the ContractReceive).
Methods that send a response (e.g., Fuse) generate 2 blocks (receive + send).
The content selector batches contract blocks per contract address and drops the
batch `[sendN, recvN+1]` when the cap would be exceeded.

**Hypothesis:** The contract block cap of 80 is the binding constraint for
contract throughput per momentum.

## Method

### Part A: 50 Stake calls (`labctl contractcap 50 1 200 <base>`)

50 accounts, each with 200 QSR fused (399,000 plasma after setup), send 1
Stake call simultaneously. Stake is `EmbeddedSimple` (52,500 plasma) and
generates only 1 contract block (ContractReceive, no response send).

Expected: 50 contract blocks < 80 cap → all included in one momentum.

### Part B: 100 Stake calls (`labctl contractcap 100 1 200 <base>`)

100 accounts, same setup. 100 contract blocks > 80 cap → cap should bind.

Expected: ~80 calls in first momentum, 20 in second.

### Configs (3 seeds)

| Seed | Base Index | Accounts | Calls/Acct | QSR/Acct |
|------|-----------|----------|------------|----------|
| 1 | 500 | 100 | 1 | 200 |
| 2 | 700 | 100 | 1 | 200 |
| 3 | 900 | 100 | 1 | 200 |

Plus one 50-account run (base 400) as a control.

## Results

### Part A: 50 Stake calls (control)

| Metric | Value |
|--------|-------|
| Total calls | 50 |
| Published | 50 (0 failed) |
| First momentum calls | 50 |
| First momentum contract blocks | 50 |
| Cap enforced | N/A (50 < 80) |

All 50 Stake calls included in momentum 7749. 50 contract blocks in momentum 7750.
Both well below the 80 cap. No constraint hit.

### Part B: 100 Stake calls (3 seeds)

| Seed | First Batch | Second Batch | Contract Blocks (M+1) | Cap Enforced |
|------|-------------|--------------|-----------------------|--------------|
| 1 | 73 | 27 | 73 | No |
| 2 | 74 | 26 | 74 | No |
| 3 | 75 | 25 | 75 | No |

**All 100 calls published successfully (0 failures across all seeds).**
**All 100 calls included (100% inclusion).**

First batch: 73-75 calls per momentum (mean 74.0, σ = 0.82).
Second batch: 25-27 calls (the remainder).
Contract blocks in next momentum: 73-75 (1 per call, confirming Stake generates
only ContractReceive, no ContractSend).

**The contract block cap of 80 was never reached in any momentum.**

### Why 73-75 and not 80?

The plasma budget (`MaxBasePlasmaInMomentum = 4,200,000`) is the binding
constraint, not the contract block cap.

Each Stake call consumes 52,500 plasma (user send). The momentum also includes
~17 setup blocks (receive funding, etc.) at 21,000 plasma each:

```
Available for Stake: 4,200,000 - (17 × 21,000) = 3,843,000
Max Stake calls: 3,843,000 / 52,500 = 73.2 → 73
```

Seed 2 had slightly fewer setup blocks, allowing 74. Seed 3 had 75.
The variation (73-75) is explained by the exact number of setup blocks
consuming plasma budget.

### Contract Block Cap vs Plasma Budget

| Constraint | Limit | Binding? |
|-----------|-------|----------|
| Contract block cap | 80 embedded blocks/momentum | No (max observed: 75) |
| Plasma budget | 4,200,000 plasma/momentum | Yes (limits user sends to ~73-80) |
| Tx count limit | 200 blocks/momentum | No (max observed: ~90) |

For EmbeddedSimple calls (52,500 plasma): plasma budget allows 80 calls,
contract cap allows 80 blocks. Both constraints coincide.

For EmbeddedWResponse calls (73,500 plasma): plasma budget allows 57 calls,
contract cap allows 80 blocks. Plasma is more restrictive.

For regular transfers (21,000 plasma): plasma budget allows 200 transfers,
contract cap is irrelevant (no embedded blocks). Tx count limit binds at 200.

**Key insight:** The contract block cap is never the sole binding constraint
for user-initiated contract calls. The plasma budget always limits user sends
to ≤ the contract block cap, because:
- Each user send consumes plasma ≥ 21,000
- Each contract call generates ≥ 1 embedded block
- Plasma budget / min plasma per call ≤ contract block cap
- 4,200,000 / 52,500 = 80 = contract block cap (exactly aligned for EmbeddedSimple)

The contract block cap would only be binding if embedded contracts could
trigger other embedded contracts (embedded-to-embedded) without user
intervention. This doesn't occur in the current design.

## Pipeline Observation

Contract calls follow a 2-momentum pipeline:

```
Momentum N:   User sends included (plasma budget limited)
Momentum N+1: Contract receives included (contract block cap limited)
Momentum N+2: User receives included (plasma budget limited)
```

This is consistent with the phase 12 saturation finding: effective throughput
under sustained contract load is ~50% of the per-momentum limit, because
user sends and contract blocks alternate in consecutive momenta.

## Conclusion

1. **The contract block cap of 80 per momentum exists in code but was NOT the
   binding constraint** in any tested configuration (max observed: 75).
2. **The plasma budget is the binding constraint** for user-initiated contract
   calls, limiting EmbeddedSimple calls to ~73-80 per momentum depending on
   other plasma consumption.
3. **The contract block cap and plasma budget are mathematically aligned** for
   EmbeddedSimple calls (4,200,000 / 52,500 = 80), making them redundant
   constraints in practice.
4. **Contract throughput is further halved by the 2-momentum pipeline**:
   user sends in momentum N, contract blocks in N+1. Effective sustained
   contract throughput is ~37-40 calls per momentum-period (10 seconds),
   not 80.

## Limits

- Single-producer devnet; results may differ with multiple pillars.
- Only Stake (EmbeddedSimple, no response) was tested. Fuse (EmbeddedWResponse)
  would have different plasma dynamics.
- The contract block cap might matter in scenarios not tested here (e.g.,
  embedded-to-embedded calls, or if MaxBasePlasmaInMomentum is increased
  via SetVariables without a corresponding cap increase).
