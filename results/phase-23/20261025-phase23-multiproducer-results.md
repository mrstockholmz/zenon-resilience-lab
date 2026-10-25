# Phase 23: Multi-Producer Devnet — 3/3 Active Pillars

**Date:** 2026-10-25
**Devnet:** chain 69, go-zenon `e89f91f`, 3 pillars all active
**Previous phases:** All used 1-active pillar (dev1 only), 30s mean inter-momentum time
**This phase:** All 3 pillars active (dev1, dev2, dev3), 10s mean inter-momentum time

## Setup

Started two additional znnd instances (`lab-devnet-pillar2`, `lab-devnet-pillar3`)
alongside the existing `lab-devnet` (pillar1). All three use the same lab-patched
genesis with 3 pillar entries. P2P via libp2p on 127.0.0.1, ports 56995-56997.

Key config changes:
- `P2PBackend: "libp2p"` forced on all three nodes (skips spork oracle, needed
  because new nodes start from genesis where libp2p spork is not yet active)
- BootstrapPeers on pillars 2/3 point to pillar 1's libp2p multiaddr
- Pillar 1 config updated to include pillars 2/3 as bootstrap peers

## Key Findings

### 1. Momentum Production Rate: Exactly 10.0s Mean

| Metric | 1-active (phase 21) | 3-active (this phase) | Ratio |
|--------|---------------------|----------------------|-------|
| Mean dt | 30.0s | 10.0s | 3.0× |
| Stdev | 22.4s | 0.0s | — |
| Min dt | 10s | 10s | 1.0× |
| Max dt | 180s | 10s | 18× |
| Distribution | Biased (30.9% at 10s) | 100% at 10s | — |

With 3/3 active pillars, every 10-second slot is filled. The deterministic shuffle
bias observed in phase 21 disappears entirely — there are no gaps because every
pillar produces its assigned slot. The distribution is a delta function at 10s.

**Formula confirmed:** BlockTime × N/K where N=3 pillars, K=active.
- N=3, K=1: 30s (phase 21)
- N=3, K=3: 10s (this phase)

### 2. Producer Distribution: Roughly Uniform

Over 25 momenta (4-minute sample):
- dev1: 8 (32%)
- dev2: 7 (28%)
- dev3: 10 (40%)

Expected: 33.3% each. The deviation is within normal variance for n=25.
Consecutive same-producer runs exist (dev3: 3 in a row, dev1/dev2: 2 in a row),
which is expected from a random shuffle.

### 3. Saturation Behavior: 200 tx/momentum Limit Still Enforced

250 self-transfers from agent account:
- Momentum 9334: 50 agent txs (90 total content)
- Momentum 9337: 200 agent txs (205 total content) — **exactly at limit**
- Momentum 9340: 50 agent txs (50 total content)

The 200 tx/momentum limit is strictly enforced even with 3 active pillars.
Excess txs are queued and included in subsequent momenta.

**Queue drain rate comparison:**
- 1-active (30s momenta): ~200 txs per 3 momenta = ~90s
- 3-active (10s momenta): ~200 txs per 3 momenta = ~30s
- **3× faster queue drain** with 3 active pillars

### 4. Transaction Latency: Unchanged

| Workload | 1-active | 3-active |
|----------|----------|----------|
| Ordinary (3 tx) | 0-1ms | 1ms |
| Burst (5 tx) | 0-1ms | 0-1ms |
| 250 tx saturation | — | p50=1ms p90=4ms |

Inclusion latency is dominated by network propagation and mempool processing,
not by momentum production rate. 3× faster momenta do not reduce per-tx latency.

### 5. Plasma Regeneration: 1 Momentum (10s vs 30s)

After depleting plasma via 50 self-transfers (1,050,000 plasma used):
- Plasma fully regenerated within 12 seconds (1 momentum + margin)
- With 1-active: regeneration took ~30s (1 momentum at 30s rate)
- With 3-active: regeneration takes ~10s (1 momentum at 10s rate)

**Effective plasma regeneration rate is 3× faster** with 3 active pillars.

### 6. Effective Throughput

| Metric | 1-active | 3-active | Ratio |
|--------|----------|----------|-------|
| Momentum rate | 0.033/s | 0.1/s | 3.0× |
| Max txs/momentum | 200 | 200 | 1.0× |
| Effective throughput | ~6.7 txs/s | ~20 txs/s | 3.0× |

The per-momentum tx limit is unchanged (200), but 3× more momentums per second
means 3× the effective network throughput.

## Implications for Prior Results

All prior phases (1-22) used 1-active pillar with 30s momenta. The core findings
are unchanged in nature but the **time scales differ by 3×**:

| Finding | 1-active | 3-active |
|---------|----------|----------|
| Plasma regeneration | 1 momentum (~30s) | 1 momentum (~10s) |
| Queue drain (200 txs) | ~90s | ~30s |
| PoW inclusion window | ~30s per momentum | ~10s per momentum |
| Saturation recovery | ~90s per batch | ~30s per batch |

**PoW transactions are most affected:** With 10s momenta, a PoW tx taking 30-60s
to compute may miss multiple momenta. The "PoW race window" is 3× tighter.
However, inclusion is still guaranteed (100% for sequential sends) because the
mempool holds txs across momenta.

**Fused plasma transactions are unaffected:** They are included in 0-1ms regardless
of momentum rate. The 3× faster momenta mean 3× more opportunities to transact.

## Raw Results

- `momentum-rate-3p.json` — 3-minute sample (18 momenta)
- `momentum-rate-3p-4min.json` — 4-minute sample (25 momenta)
- `ordinary-3tx-3p.json` — basic 3-tx ordinary workload
- `burst-5tx-3p.json` — basic 5-tx burst workload
- `saturate-250tx-3p.json` — 250-tx saturation test (single account)
- `contentmix-3self-3p-seed0.json` — 3-account content mix
- `powlat-3acct-3p.json` — PoW latency (accounts already had plasma, no PoW needed)

## Reproduction

1. Start pillar 1: `znnd --data ~/lab-devnet` (with P2PBackend=libp2p, BootstrapPeers including pillars 2/3)
2. Start pillar 2: `znnd --data ~/lab-devnet-pillar2` (with P2PBackend=libp2p, BootstrapPeers=[pillar1])
3. Start pillar 3: `znnd --data ~/lab-devnet-pillar3` (with P2PBackend=libp2p, BootstrapPeers=[pillar1])
4. Wait for sync: `labctl info` until momentum_height matches on all three
5. Run tests: `labctl ordinary 3 1`, `labctl burst 5 1`, etc.

## Caveats

- Single-machine deployment: all 3 nodes share CPU, memory, and disk I/O.
  Timing measurements may be affected by resource contention.
- The deterministic shuffle bias (phase 21 finding) was observed with 1-active
  pillar. With 3-active, there are no gaps to be biased about — every slot is
  filled. The bias would re-emerge if some slots were empty (e.g., 2-active).
- This test does not measure network latency effects (all nodes on localhost).
  Real-world multi-producer deployments would have network propagation delays.
