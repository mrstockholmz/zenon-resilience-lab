# Phase 10 — RPC API Latency Under Load Results

**Date:** 2026-10-23
**Status:** done (3 burst sizes: 100, 200, 500 txs)

## Question

When the node is processing a high volume of transactions, does RPC API
responsiveness degrade? This matters for node operators, wallet apps, and
any service that depends on timely RPC responses during network congestion.

## Background

Phases 2-9 measured transaction inclusion, plasma mechanics, and PoW latency.
None tested whether the node's RPC endpoints remain responsive while the
node is busy processing transactions. On mainnet, a wallet or explorer
querying the node during a burst should know whether to expect delays.

## Method

`suite/rpcload.sh <num_burst_txs>` — measures latency of 5 common RPC
endpoints during baseline (no load) and under a concurrent `labctl burst`
workload. Each measurement is 20 sequential requests; percentiles are
computed from the 20 samples.

**Endpoints tested:**
- `ledger.getFrontierMomentum` — most common query (node state)
- `ledger.getFrontierAccountBlock` — account state lookup
- `embedded.plasma.get` — plasma balance query
- `embedded.plasma.getRequiredPoWForAccountBlock` — PoW difficulty estimate
- `stats.get` — node statistics

**Burst sizes:** 100, 200, 500 transactions (all from the fused agent account,
so no PoW delay — the node is maximally busy processing).

**Hardware:** i7-4770, nice -n 10, RPC over localhost (127.0.0.1:57997).

## Key Findings

### 1. RPC latency is essentially unchanged under load

| Burst | Endpoint | Baseline p50 | Under-load p50 | Δ |
|-------|----------|-------------|----------------|---|
| 100 | getFrontierMomentum | 9ms | 10ms | +1ms |
| 100 | getFrontierAccountBlock | 9ms | 9ms | 0ms |
| 100 | plasma.get | 9ms | 9ms | 0ms |
| 100 | getRequiredPoW | 9ms | 10ms | +1ms |
| 100 | stats.get | 9ms | 9ms | 0ms |
| 200 | getFrontierMomentum | 9ms | 10ms | +1ms |
| 200 | getFrontierAccountBlock | 9ms | 10ms | +1ms |
| 200 | plasma.get | 9ms | 10ms | +1ms |
| 200 | getRequiredPoW | 9ms | 10ms | +1ms |
| 200 | stats.get | 9ms | 9ms | 0ms |
| 500 | getFrontierMomentum | 10ms | 11ms | +1ms |
| 500 | getFrontierAccountBlock | 10ms | 10ms | 0ms |
| 500 | plasma.get | 10ms | 10ms | 0ms |
| 500 | getRequiredPoW | 9ms | 10ms | +1ms |
| 500 | stats.get | 9ms | 9ms | 0ms |

**p99 latencies** show a similar pattern: 10-12ms baseline, 10-15ms under load.
The maximum observed under-load p99 was 15ms (getFrontierMomentum at 500 txs).

### 2. No endpoint is disproportionately affected

All 5 endpoints show the same pattern: a 0-1ms increase in p50 latency
under load. The node does not prioritize or deprioritize any particular
RPC endpoint during transaction processing.

### 3. The node is I/O-bound, not CPU-bound, for RPC

The 9-10ms baseline latency is dominated by HTTP round-trip overhead on
localhost, not by node processing. Even with 500 transactions being
processed concurrently, the node's event loop has sufficient spare
capacity to answer RPC queries without queuing.

## Implications for Mainnet

1. **RPC endpoints remain responsive during transaction bursts.** Wallets,
   explorers, and monitoring tools should not experience RPC timeouts or
   significant latency increases during high-demand periods.

2. **No need for RPC rate-limiting under load.** The node handles both
   transaction processing and RPC queries without degradation. Rate limits
   (if any) should be based on abuse-prevention concerns, not performance.

3. **Node operators can safely run RPC on the same instance as the
   producer.** There is no evidence that high transaction volume interferes
   with RPC responsiveness on a single-producer devnet.

## Limits

- Single-producer devnet; mainnet with 97 pillars and network propagation
  may show different results.
- Only 5 endpoints tested. Less common endpoints (e.g., contract state
  queries, historical data) may behave differently.
- The burst workload is from a single fused account (no PoW). A PoW-heavy
  workload might consume more CPU and affect RPC latency.
- The measurement is 20 samples per endpoint; larger samples would give
  tighter percentile estimates.
- All RPC queries are over localhost. Network RPC (remote clients) would
  have additional latency unrelated to node load.

## Raw Data

- `rpcload-100tx.json` — 100-tx burst (baseline + under-load)
- `rpcload-200tx.json` — 200-tx burst (baseline + under-load)
- `rpcload-500tx.json` — 500-tx burst (baseline + under-load)
