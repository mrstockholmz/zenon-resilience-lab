# Phase 25 — ZVM relayer batches: NoM L1 plasma cost and throughput bounds

**Date:** 2026-11-05
**Substrate:** ZVM devnet L1 — go-zenon dev ruleset, chain 69, dynamic plasma active
(v2 momenta), single pillar, 10 s momenta. L1 RPC: container `zvm-devnet-l1-rpc-1`
(http://172.18.0.8:35997 from the host), faucet `zvm-devnet-faucet-1` (:8549).
**Probe:** `probe/` in this directory (Go, uses zvm-core's public devnet mnemonic;
account = mnemonic index 400+, disposable).

## Question

What does it cost to settle ZVM rollup batches on NoM L1, and where does
throughput bind?

The ZVM relayer posts batches as plain NoM user sends (block type 2) to the
inbox address `z1zvmxxxxxxxxxxxxxxxxxxxxxxxxxxxxxafdtd6`, amount 0, data =
compressed batch payload (zvm-relayer relayer.go:906-927; zvm-core
derive/messages.go). Data cap per block: 16,384 B (zvm-core znn/block.go:22).

## Measured (probe, 2026-11-05, chain 69, floor price 1000)

Ladder regression assertion **R21a PASS** (5 sizes exact); burst assertion
**R21b PASS** (max 6 and 7 blocks per momentum across two runs, ≤ 7 bound,
9 blocks always spanning ≥ 2 momenta). Checker: `check-phase25.py`.

| data bytes | planned plasma (21000+68·B) | base | used | fused | PoW difficulty | result |
|---|---|---|---|---|---|---|
| 0 | 21,000 | 21,000 | 21,000 | 21,000 | 0 | formula exact |
| 1,024 | 90,632 | 90,632 | 90,632 | 90,632 | 0 | formula exact |
| 4,096 | 299,528 | 299,528 | 299,528 | 299,528 | 0 | formula exact |
| 8,192 | 578,056 | 578,056 | 578,056 | 578,056 | 0 | formula exact |
| 16,384 | 1,135,112 | 1,135,112 | 1,135,112 | 1,135,112 | 0 | formula exact |
| 16,385 | — | — | — | — | — | **rejected**: "data is 16385 bytes, the limit is 16384" |

(probe-result.json, run 2026-11-05; account fused to 1,000 QSR = 2,100,000 plasma;
all sends fully fused, zero PoW.)

Partial-fusion + PoW top-up (measured on the first ladder run, 8,192 B block with
only 200 QSR fused = 420,000 plasma): fused = 420,000, PoW difficulty =
237,084,000, used = 578,056 = planned. A fused account that is short of plasma
covers the remainder with PoW; total used plasma is unchanged.

Burst binding measurement (burst-result.json, two runs):

- Run 1: 9 × 8 KiB blocks (3 accounts × 3, 2.1 M plasma each) → **7 blocks in
  momentum 111562** (4.046 M plasma ≤ 4.2 M budget), 2 in 111565.
- Run 2: 6 blocks in momentum 111609, 3 in 111612 (max 6 ≤ 7).
- Both runs agree with the budget bound: floor(4,200,000 / 578,056) = 7 blocks
  per momentum at floor price. Nine blocks never fit in one momentum
  (9 × 578,056 = 5.2 M > 4.2 M).

Also confirmed live: plasma regenerates to full on confirmation (two-counter
system): currentPlasma returned to max between confirmed sends.

## Model

Constants (zvm-core znn/block.go:17-25; lab v0.1 phases 2/18 confirmed on chain 99/100,
this phase confirms on chain 69):

- `plasma(block) = 21,000 + 68 × dataBytes` at floor price (fusion price 1000).
- Full batch block (16,384 B): **1,135,112 plasma**.
- Per-momentum base plasma budget: **4,200,000** → at floor price at most
  **3 full batch blocks per momentum** (49,152 B compressed payload).
  The relayer's own footprint calculator agrees: `blocksPerMomentum=3` at
  `budgetPerMomentum=3,780,000` (90% policy ceiling of 4.2 M).

Throughput bound at floor price:

- L1 data settled per momentum: ≤ 49,152 B compressed.
- Per L2 tx: ~100–130 B compressed typical (span codec, zvm-core derive/span.go;
  65 B signature dominates). **Not measured here — the devnet relayer lane has
  seen zero batches, so the per-tx compressed size is a code-constant estimate,
  not an observation.**
- ⇒ ~380–490 L2 txs settled per 10 s momentum ≈ **38–49 L2 txs/s** at 10 s
  momenta; scales linearly with momentum rate.

Relayer provisioning:

- 5,000 QSR fused per relayer account = 10.5 M plasma = in-flight budget for
  ~9 full blocks at floor price; at price 1.8× only ~5. The relayer's rebidder
  exists precisely because fused blocks can be stranded by a price rise
  (zvm-relayer relayer.go:47-48,1462-1469,943-973). 8 accounts → ~36-72 full
  blocks in flight network-wide.
- Token cost of batch posting: **zero**. Sends carry amount 0; plasma is a rate
  limit (QSR remains fused and recoverable), not a burn. The L1 cost of a batch
  is the opportunity cost of fused QSR plus PoW electricity for any unfused part.

Where throughput binds, in order:

1. **Per-momentum plasma budget (4.2 M)** — the hard protocol bound: 3 full
   blocks per momentum at floor price. Burst measurement above confirms.
2. **Dynamic plasma repricing** — sustained usage above the target (>50% of
   budget = 2.1 M fused plasma per momentum) raises the price ≤10%/momentum;
   at price p the budget buys 3.7 × 1000/p full blocks. Throughput degrades
   gracefully; it does not hard-stop. (Mechanics measured in lab phase 14/15;
   not re-measured with batch traffic here.)
3. **Per-account fused plasma** — caps in-flight blocks per relayer account;
   mitigated by the relayer's 8-account sharding.
4. Not binding: the 200 tx/momentum count cap (3 batch blocks ≪ 200), the 280
   content cap, the 80 contract-block cap (batch posts are user sends).

## What a devnet cannot show

- **Real price discovery.** One pillar, one operator, no competing demand. The
  repricing path under genuine contention (who gets the 3 block slots, how fast
  the price really climbs with multi-relayer competition) needs a multi-operator
  network.
- **Compressed size of real L2 traffic.** The devnet relayer lane has processed
  zero batches; per-tx byte cost (~100–130 B) is from code constants and
  upstream OP-stack analogy, not from observed traffic.
- **Mempool and p2p behavior at scale**: 16 KiB txs across many real peers,
  propagation effects, mempool eviction under load.
- **Relayer economics against real variance**: mainnet momentum-rate variance,
  pillar production failures, and the unfuse time-lock all change the QSR
  opportunity-cost calculation.
- **Multi-relayer fairness**: 8 accounts here belong to one relayer; competing
  relayers splitting the 3-slot budget is untested.

## Reproduce

```bash
# from this directory; requires the ZVM devnet stack running (docker)
cd probe
go build .
./zvmplasmaprobe -l1 http://172.18.0.8:35997 -faucet http://172.18.0.3:8549 -index 400   # ladder
./zvmplasmaprobe -burst -l1 http://172.18.0.8:35997 -faucet http://172.18.0.3:8549 -index 400  # binding
```

Raw output: `probe-result.json`, `probe-run.log`, `burst-result.json`, `burst-run.log`.
