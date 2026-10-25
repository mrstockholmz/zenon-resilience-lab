# Current Findings — v0.1

**Release date:** 2026-10-25
**Pinned go-zenon:** `e89f91f68b922a65370b7ffd9cfe860045a99b29`
**Plasma mode:** static <30 / dynamic >=30 (devnet genesis spork 0000...0002, enforcementHeight 30)

## What this lab measures

Reproducible experiments on how Zenon behaves under automated demand. First topic:
Plasma allocation and transaction demand patterns — confirmation latency, resource
cost, and whether a lightly provisioned account can still transact.

## Current conclusions (one per topic)

### Plasma mechanics
- Each simple self-transfer consumes exactly 21,000 plasma. Data payload adds 68 plasma/byte.
  Contract calls cost 52,500 plasma (EmbeddedSimple). All confirmed against go-zenon source.
- Plasma is a per-momentum rate limit, not a permanent budget. After confirmation,
  the full fused amount is available again. 100 QSR = 210,000 plasma = 10 simple txs/momentum.
- Plasma regenerates from 0 to full within 1 momentum (two-counter system: committed/unconfirmed).

### PoW fallback
- Fused plasma eliminates PoW entirely. 5 sequential txs: 0.02s with plasma vs 41–79s without.
- PoW computation dominates latency (3.3–75.8s, geometric distribution). Inclusion itself is near-instant (0–34ms).
- Single-account burst rejection is caused by PoW delay creating account-chain conflicts
  on a stale frontier — not by any protocol-level rejection rule.

### Throughput limits
- The 200 tx/momentum limit is strictly enforced. Excess is queued, not rejected.
  Queue drain: ~200 txs per 3 momenta (2 empty momenta between batches).
  Effective sustained throughput: ~67 txs/momentum, not 200.
- The 280 content cap is a correctness bound, not a throughput limit. Plasma budget
  (4.2M) limits user blocks to exactly 200.
- Contract block cap of 80/momentum is NOT binding. Plasma budget limits contract calls
  to ~73–75/momentum (52,500 plasma each). Cap and budget are mathematically aligned (4.2M/52.5K=80).

### Fee market / prioritization
- At floor prices (all txs pay minimum), there is NO plasma-based prioritization.
  Selection is FIFO by arrival time; fused txs arrive first only because they skip PoW.
- When prices differ, the content selector sorts by price-per-base-plasma.
  Overpaying buys priority. A dormant fee market exists.

### Dynamic Plasma repricing
- Repricing fires when momentum-level plasma usage exceeds the target (>50 fused txs/momentum).
- Formula: `change = price * (used - target) / target / 20`, capped at ±10%/momentum.
- Price decays ~5% per empty momentum, returning to floor (1000) after 2 empty momenta.
- workPrice repricing via PoW alone is impractical on typical hardware (3–76s per tx,
  needs 50+ parallel CPUs to trigger).

### Mempool and validation
- Per-account mempool cap: 500 uncommitted blocks. Strictly enforced.
- Frontier conflict resolution: price-then-hash rule (higher price wins; tie → smallest hash).
- All 13 negative validation cases correctly rejected with informative errors.
  Zero-amount sends are valid (boundary case).

### Momentum production
- Rate scales as BlockTime × N/K (N pillars, K active). 3 pillars, 1 active: 30s mean.
  3 pillars, 3 active: 10s mean, near-zero stdev.
- The deterministic shuffle (math/rand with height seed) is biased toward consecutive
  slots for small permutation spaces. Distribution does NOT match uniform shuffle model.
- Momentum timestamps are slot start times (deterministic from genesis), not wall-clock.
- Transaction load does not affect production rate.

### RPC resilience
- RPC latency unchanged under burst load (100/200/500 txs). p50: 9–10ms → 10–11ms under load.
  Node is I/O-bound for RPC, not CPU-bound.

## Superseded claims

| # | Original claim | Superseded by | Why |
|---|---|---|---|
| 13 | No plasma-based prioritization under saturation | Phase 14 | Phase 13 tested only same-price txs. When prices differ, the content selector DOES prioritize by price-per-base-plasma. |
| 8 | Plasma regenerates over time (rate TBD) | Phase 15 | Phase 8 implied time-based regeneration. Phase 15 confirmed: regeneration is on confirmation, not over time. Full regen within 1 momentum. |
| 13 (finding) | 20–40% inclusion for unfused burst | Phase 11, 24 | Original unfused burst tests used a single account with 5 simultaneous txs (goroutines on stale frontier). Multi-account and multi-producer tests show 100% inclusion for unfused txs when sent sequentially or with sufficient time between submissions. |

## Single-producer limits

All phase 2–20 results were obtained on a **single-producer devnet** (1 pillar, 30s momenta).
These results describe resource mechanics only. They say nothing about decentralization,
fairness, or consensus security. Multi-producer results (phases 21–24) used a 3-pillar
devnet with 10s momenta and confirmed the core findings hold under faster production.

Specific limits of single-producer results:
- Queue drain rate and effective throughput scale with momentum rate (3× faster on 10s momenta).
- Plasma regeneration rate scales with momentum rate (regenerates per momentum, not per second).
- PoW latency is CPU-bound, not network-bound — unchanged across momentum rates.
- No contention between producers, no fork resolution, no cross-pillar latency.

## Reproduction

See `suite/README.md` for full instructions. One command: `./run-suite.sh all`.
Regression assertions: `python3 suite/check-regressions.py` (R1–R20).
Reset procedure: see `docs/RESET.md`.

## Honesty notes

- All results are from an isolated devnet with disposable keys. No mainnet or testnet data.
- Simulated or estimated numbers are never presented as observed ones.
- Failed, skipped, and unverified steps are stated as such in result files.
- The regression checker reports PASS / FAIL / NOT TESTED — it does not claim coverage
  it does not have.
