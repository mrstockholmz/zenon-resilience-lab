# Phase 2 Local Verification — 2026-10-25

## Summary
Re-ran phase 2 workloads on the multi-producer devnet (3 pillars, 10s momenta)
to verify original findings hold under the faster momentum regime.

## Environment
- **Devnet:** 3 pillars, all active (phase 23 setup)
- **Momentum height:** ~12048 (dynamic plasma regime, >= 30)
- **Agent plasma:** 22,995,000 (fused, ~1095 QSR)
- **Machine:** nummer13 (i7-4770, 32GB RAM)

## Workloads

### Ordinary (3 txs, 1 ZNN each)
- **Result:** 3/3 included, 0 rejected, 0 unresolved
- **PoW count:** 0 (plasma covered all txs)
- **Latency:** p50=1ms, p90=1ms, p99=1ms
- **Duration:** 0.015s
- **Plasma after:** 22,932,000 (3 × 21,000 = 63,000 consumed)

### Burst (5 txs, 1 ZNN each, concurrent)
- **Result:** 5/5 included, 0 rejected, 0 unresolved
- **PoW count:** 0 (plasma covered all txs)
- **Latency:** p50=1ms, p90=1ms, p99=1ms
- **Duration:** 0.054s
- **Plasma after:** 22,827,000 (5 × 21,000 = 105,000 consumed)

### Probe (30s, 3 txs)
- **Result:** 3/3 included, 0 rejected, 0 unresolved
- **Latency:** p50=3ms, p90=4ms, p99=4ms
- **Duration:** 30.15s (spans 3 momenta)

## Comparison with Original Phase 2 (2026-10-21, 1-producer 30s momenta)

| Metric | Original (30s) | Local (10s) | Change |
|--------|----------------|-------------|--------|
| Ordinary inclusion | 3/3 | 3/3 | Same |
| Ordinary latency | 1ms | 0-1ms | Same |
| Burst inclusion | 5/5 (fused) | 5/5 (fused) | Same |
| Burst latency | 0-1ms | 0-1ms | Same |
| PoW count | 0 | 0 | Same |

## Conclusion
Phase 2 findings are **confirmed** on the multi-producer devnet. Fused plasma
eliminates PoW and enables instant inclusion regardless of momentum rate.
The 3× faster momentum production (10s vs 30s) does not change the
fundamental behavior: plasma covers all transactions, latency remains 0-1ms.

## Raw Data
- `local-verification/dynamic-ordinary-fused-local-r1.json`
- `local-verification/dynamic-burst-fused-local-r1.json`
- `local-verification/dynamic-probe-fused-local-r1.json`
