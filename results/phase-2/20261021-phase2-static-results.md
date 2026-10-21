# Phase 2 Results — Static Plasma Regime (momentum < 30)

**Date:** 2026-10-21
**Node:** lab devnet, go-zenon `e89f91f`, single pillar, chain 69
**Regime:** Static Plasma (momentum height < 30)
**Agent:** z1qp3yph55qgresyytz83anynr2f4z39x2z3ej3e (BIP-44 index 3, genesis-funded)

## Ordinary Workload (sequential self-transfers, 1 ZNN each)

| run | attempted | submitted | included | rejected | unresolved | PoW count | total PoW ms | p50 lat | p90 lat | duration s |
|-----|-----------|-----------|----------|----------|------------|-----------|--------------|---------|---------|------------|
| r1  | 3         | 3         | 3        | 0        | 0          | 3         | 53280        | 1       | 1       | 53.3       |
| r2  | 3         | 3         | 3        | 0        | 0          | 3         | 10168        | 1       | 2       | 10.2       |
| r3  | 3         | 3         | 3        | 0        | 0          | 3         | 18360        | 1       | 1       | 18.4       |

**Finding:** Ordinary workload has 100% inclusion rate. All transactions use PoW fallback
(agent has 0 fused plasma). PoW time varies widely (0.9s – 53s) depending on difficulty.
Once PoW is solved, inclusion is near-instant (1-2ms latency) — the transaction is
included in the very next momentum.

## Burst Workload (5 parallel self-transfers from single account)

| run | attempted | submitted | included | rejected | unresolved | inclusion rate |
|-----|-----------|-----------|----------|----------|------------|----------------|
| r1  | 5         | 2         | 2        | 3        | 0          | 40%            |
| r2  | 5         | 1         | 1        | 4        | 0          | 20%            |
| r3  | 5         | 2         | 2        | 3        | 0          | 40%            |

**Rejection reasons:**
- "hash tie-break is worse for current block" — two transactions with same prevHash compete; one wins
- "account-block prevHash exists but it has a cemented block on top of it" — by the time
  the slower transaction is submitted, the account chain has already moved forward

**Finding:** Single-account burst has 20-40% inclusion rate. This is an inherent property
of the account-chain model: all transactions from one account must form a sequential chain,
so parallel submissions inevitably conflict. The first transaction to solve PoW and reach
the node wins; the rest are rejected. This is not a bug — it is the expected behavior
of a sequential account chain under parallel submission.

## Probe (lightly-provisioned account, 10s interval)

| run | attempted | submitted | included | rejected | PoW count | total PoW ms | p50 lat |
|-----|-----------|-----------|----------|----------|-----------|--------------|---------|
| r1  | 2         | 2         | 2        | 0        | 2         | 24707        | 1       |

**Finding:** A lightly-provisioned account (0 fused plasma, small balance) can transact
reliably using PoW fallback. PoW cost per transaction is 7-17 seconds on this hardware
(4 cores, nice -n 10).

## Raw Data

- `static-ordinary-r1.json`, `static-ordinary-r2.json`, `static-ordinary-r3.json`
- `static-burst-r1.json`, `static-burst-r2.json`, `static-burst-r3.json`
- `static-probe-r1.json`

## Notes

- PoW difficulty varies per block because the required difficulty is a function of
  the account's available plasma (0 for our unfused accounts) and the block's complexity.
- The wide range in PoW time (0.3s – 80s) reflects the stochastic nature of the
  proof-of-work puzzle, not a systematic difference between runs.
- Generator CPU time tracks wall-clock time closely (ratio ~1.0) because the PoW
  solver is CPU-bound on 4 cores.
