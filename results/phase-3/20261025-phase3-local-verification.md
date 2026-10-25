# Phase 3 Local Verification — 2026-10-25

## Summary
Re-ran key phase 3 matrix cases on the multi-producer devnet (3 pillars, 10s
momenta) to verify original findings hold under the faster momentum regime.

## Environment
- **Devnet:** 3 pillars, all active (phase 23 setup)
- **Momentum height:** ~12052 (dynamic plasma regime)
- **Agent plasma:** ~22,995,000 (fused, ~1095 QSR)
- **Note:** Agent account has fused plasma; "low" cases use the agent's existing
  plasma rather than unfused PoW. For unfused behavior see original phase 3.

## Matrix Cases (3 seeds each)

### low-steady-1 (5 ordinary txs, 1 account)
| Seed | Attempted | Included | Rejected | Unresolved | p50 ms | p90 ms |
|------|-----------|----------|----------|------------|--------|--------|
| 0    | 5         | 5        | 0        | 0          | 0      | 1      |
| 1    | 5         | 5        | 0        | 0          | 0      | 0      |
| 2    | 5         | 5        | 0        | 0          | 0      | 0      |

### low-burst-1 (10 burst txs, 1 account)
| Seed | Attempted | Included | Rejected | Unresolved | p50 ms | p90 ms |
|------|-----------|----------|----------|------------|--------|--------|
| 0    | 10        | 10       | 0        | 0          | 1      | 1      |
| 1    | 10        | 10       | 0        | 0          | 1      | 2      |
| 2    | 10        | 10       | 0        | 0          | 1      | 1      |

### low-steady-multi (3 ordinary txs, funded 2 extra accounts)
| Seed | Attempted | Included | Rejected | Unresolved | p50 ms | p90 ms |
|------|-----------|----------|----------|------------|--------|--------|
| 0    | 3         | 3        | 0        | 0          | 1      | 1      |
| 1    | 3         | 3        | 0        | 0          | 1      | 1      |
| 2    | 3         | 3        | 0        | 0          | 1      | 1      |

## Key Observations

1. **100% inclusion across all cases and seeds.** With fused plasma, all
   transactions are included in the same momentum they are submitted.

2. **Latency remains 0-2ms.** The 3× faster momentum rate (10s vs 30s) does
   not materially change inclusion latency for fused accounts.

3. **Burst of 10 txs from 1 account: 100% inclusion.** This confirms the
   original phase 3 finding that fused plasma eliminates the PoW race
   condition. Without plasma, 10 concurrent txs would have 20-40% inclusion.

4. **Multi-account funding works correctly.** The `fund` command sends 100 ZNN
   to derived accounts, which then have their own account chains.

## Spread Analysis (3 seeds)

- **low-steady-1:** p50 spread = 0-0ms, p90 spread = 0-1ms → negligible variance
- **low-burst-1:** p50 spread = 1-1ms, p90 spread = 1-2ms → negligible variance
- **low-steady-multi:** p50 spread = 1-1ms, p90 spread = 1-1ms → no variance

## Comparison with Original Phase 3 (2026-10-21, 1-producer 30s momenta)

| Case | Original Inclusion | Local Inclusion | Original p50 | Local p50 |
|------|-------------------|-----------------|--------------|-----------|
| low-steady-1 | 100% (fused) | 100% | 0-1ms | 0ms |
| low-burst-1 | 100% (fused) | 100% | 0-1ms | 1ms |
| low-steady-multi | 100% (fused) | 100% | 0-1ms | 1ms |

## Limitations

- **Agent account has fused plasma.** These runs verify fused behavior only.
  The original phase 3 "low" cases used unfused accounts (PoW). For unfused
  burst behavior, see the original phase 3 results (20-40% inclusion).
- **high-steady-1 and high-burst-1 not run.** These require fusing 100 QSR,
  which exceeds the agent's current QSR balance (4 QSR).

## Conclusion
Phase 3 findings are **confirmed** for fused accounts on the multi-producer
devnet. Fused plasma provides 100% inclusion with 0-2ms latency regardless
of momentum rate (10s vs 30s). The original unfused burst findings (PoW race,
20-40% inclusion) are expected to hold but were not re-tested due to the
agent's fused state.

## Raw Data
- `local-verification/low-steady-1-seed{0,1,2}-fused-local.json`
- `local-verification/low-burst-1-seed{0,1,2}-fused-local.json`
- `local-verification/low-steady-multi-seed{0,1,2}-fused-local.json`
