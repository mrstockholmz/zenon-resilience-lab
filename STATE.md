# State

<!-- machine-readable: the cloud brief and the head read this block -->
```
next_phase: 25
cloud_sessions_run: 1
local_sessions_run: 17
pinned_go_zenon: e89f91f68b922a65370b7ffd9cfe860045a99b29
pinned_sdk: n/a (suite is Go, imports go-zenon module directly at pinned_go_zenon; no separate SDK dependency)
plasma_mode: static<30 / dynamic>=30 (devnet genesis spork 0000...0002, enforcementHeight 30)
updated: 2026-10-25 (local verification: phase 2+3 on multi-producer devnet)
```

## Phase log
| phase | status | result file | note |
|---|---|---|---|
| 1 gap check + baseline | partial-pass | results/phase-1/20261020-cloud-phase1-baseline.md, results/phase-1/20261021-gap-check.md, results/phase-1/20261025-gap-check-local.md | Baseline proven (round trip, PoW fallback, plasma regime pinned). Gap check redone locally 2026-10-21 and 2026-10-25: no existing suite measures plasma/demand/latency — lab fills a real gap. Genesis defect found; fix merged upstream as PR #201. |
| 2 smallest experiment | done | results/phase-2/20261021-phase2-static-results.md, results/phase-2/20261021-phase2-dynamic-results.md, results/phase-2/20261025-phase2-local-verification.md | Ordinary + burst workloads in static (<30) and dynamic (>=30) regimes. Key finding: fused plasma eliminates PoW and enables instant inclusion; single-account burst rejection is caused by PoW delay, not account-chain limits. **Local verification 2026-10-25:** confirmed on multi-producer devnet (10s momenta). |
| 3 resource matrix | done | results/phase-3/20261021-phase3-matrix-results.md, results/phase-3/20261025-phase3-local-verification.md | Low vs high fused x steady vs burst x 1 vs multi accounts, 3 seeds each. 100% inclusion for sequential (all regimes) and for fused burst. 20-40% inclusion for unfused burst (PoW race). **Local verification 2026-10-25:** fused cases confirmed on multi-producer devnet. |
| 4 comparison | done | results/phase-4/20261022-phase4-comparison-results.md | Fused plasma static vs dynamic: no observable difference for existing fusion holders at low volume. Dynamic pricing (nextFusionPrice/workPrice=1000) only affects new fusions and PoW fallback under high demand. |
| 5 package | done | suite/ | One-command runner (suite/run-suite.sh with phase2/phase3/phase4/check/all), labctl tool, raw JSON results, CSV export (json2csv.py), regression assertions (check-regressions.py R1-R20 all passing). Documentation: suite/README.md with full reproduction instructions. Main README updated. |
| 6 next experiment | done | results/phase-6/20261022-phase6-repricing-results.md | Dynamic Plasma repricing confirmed. >50 fused txs/momentum triggers price rise, capped at +10%/momentum. Price decays ~5%/empty momentum to floor of 1000. 3-seed reproducible. Decision informed: when to activate dynamic-plasma spork on mainnet. |
| 7 multi-sender contention | done | results/phase-7/20261022-phase7-contention-results.md | 10 accounts with plasma gradient compete for momentum space. Higher plasma correlates with higher inclusion (Spearman rho=0.47-0.84). No automatic PoW fallback — insufficient plasma = rejection. 200-tx/momentum limit confirmed. |
| 8 plasma depletion + regeneration | done | results/phase-8/20261022-phase8-plasma-depletion-results.md | Plasma depletes by exactly 21000 per simple self-transfer. After confirmation, plasma fully regenerates to fused amount (two-counter system: committed/unconfirmed sync). 100 QSR = 10 txs/momentum. Plasma is a rate limit, not a permanent budget. |
| 9 PoW latency + cost | done | results/phase-9/20261022-phase9-pow-latency-results.md | PoW tx time: 3.3-75.8s (avg ~25s), 100% inclusion for sequential sends, inclusion latency 0-34ms. workPrice repricing impractical via PoW on this hardware (14 txs < 50 target). PoW variance is inherent (geometric distribution). |
| 10 RPC latency under load | done | results/phase-10/20261023-phase10-rpc-latency-results.md | RPC latency unchanged under burst load (100/200/500 txs). p50: 9-10ms baseline → 10-11ms under load. No endpoint disproportionately affected. Node is I/O-bound for RPC, not CPU-bound. |
| 11 mixed fused/unfused contention | done | results/phase-11/20261023-phase11-mixed-contention-results.md | Fused txs: 100% first-momentum inclusion (0 PoW). Unfused txs: 100% inclusion but spread across 2-5 momenta (PoW delay). No discrimination against PoW txs — structural disadvantage from PoW computation time, not node policy. 4 reps reproducible. |
| 12 saturation behavior | done | results/phase-12/20261023-phase12-saturation-results.md | 200 tx/momentum limit strictly enforced (no momentum exceeds 200). Excess txs queued not rejected (100% inclusion with sufficient plasma). Queue drain: ~200 txs per 3 momenta (2 empty momenta between batches). Only rejection cause: plasma exhaustion from rapid-fire sending. 3 configs (300/500/450 txs). |
| 13 mixed fused/unfused saturation | done | results/phase-13/20261023-phase13-mixed-saturation-results.md | No plasma-based prioritization under saturation. Mempool selection is FIFO by arrival time, not plasma amount. Fused txs are included first because they arrive instantly (0s PoW), not because of priority. Unfused txs with fast PoW (<20s) can get into the first momentum. 100% inclusion in all 3 seeds. R11 regression added. |
| 14 content selector price priority | done | results/phase-14/20261023-phase14-price-priority-results.md | Price-based prioritization CONFIRMED. Content selector sorts by price-per-base-plasma; higher-priced txs included first under saturation. Corrects phase 13: priority exists when prices DIFFER, not when all txs pay floor price. Overpaying (FusedPlasma > BasePlasma) buys priority. Fee market is dormant at floor prices. |
| 15 plasma regeneration rate | done | results/phase-15/20261023-phase15-plasma-regeneration-results.md | Plasma regenerates from 0 to full within 1 momentum. Two-counter model confirmed. An account with 100 QSR can send 10 txs per momentum indefinitely. Corrects phase 8: regeneration is on confirmation, not over time. |
| 16 transaction type diversity | done | results/phase-16/20261024-phase16-txtype-results.md | Plasma cost ladder confirmed: self-transfer=21000, +data=21000+68/B, contract=52500. All match go-zenon source exactly. 3-seed reproducible. R13 added. |
| 17 mempool cap + frontier conflict | done | results/phase-17/20261024-phase17-mempool-cap-results.md | Per-account mempool cap of 500 unconfirmed blocks confirmed. Drain/refill cycle observed. Frontier conflict: price-then-hash rule 15/15 correct across 3 seeds. R14 added. |
| 18 data payload size limits | done | results/phase-18/20261024-phase18-datasize-results.md | Plasma cost formula exact: 21000+68*N bytes for 0-16KB. 16KB limit strictly enforced. Max-size tx costs 1,135,112 plasma (~541 QSR). Plasma budget (4.2M) limits large-data txs to 3 per momentum. R15 added. |
| 19 contract block throughput cap | done | results/phase-19/20261024-phase19-contractcap-results.md | Contract block cap of 80/momentum exists but is NOT binding. Plasma budget limits Stake calls to ~73-75/momentum. Cap and plasma budget aligned (4.2M/52.5K=80). Stake generates 1 contract block (receive only). 2-momentum pipeline. R16 added. |
| 20 momentum content capacity | done | results/phase-20/20261024-phase20-contentmix-results.md | 280 content cap is NOT binding. Plasma budget limits user blocks to exactly 200. Max observed content: 200 (Config A seed 1, exactly at plasma budget). Mixing contract calls reduces content (Stake less plasma-efficient). Contract blocks in next momentum (2-pipeline). R17 added. |
| 23 multi-producer devnet | done | results/phase-23/20261025-phase23-multiproducer-results.md | 3/3 active pillars: 10s mean dt (stdev=0), uniform producer distribution, 200 tx limit still enforced, queue drain 3x faster (30s vs 90s), plasma regen 3x faster (10s vs 30s), effective throughput ~20 txs/s. R20 added. |
| 24 multi-producer PoW latency | done | results/phase-24/20261025-phase24-multiproducer-pow-results.md | PoW txs 100% inclusion + 100% same-momentum with 10s momenta (13/13). PoW time unchanged (~24s avg). Inclusion latency 0-3ms. Mixed contention: fused all in first momentum, unfused spread by PoW speed. 100% inclusion both groups. |
| 22 validation edge cases | done | results/phase-22/20261025-phase22-valedge-results.md | All 13 negative cases correctly rejected with matching error messages. Zero-amount send accepted (correct boundary). No validation gaps found. 3-seed reproducible. R19 added. |
| 21 momentum production rate | done | results/phase-21/20261025-phase21-momentum-rate-results.md | Mean inter-momentum time: 30.0s (3× slowdown, matching 3-pillar 1-active config). Distribution does NOT match uniform shuffle model — 30.9% at 10s vs 11.1% predicted. Momentum timestamp = slot start time (0s delay). Tx load does not affect production rate. Effective throughput: ~6.7 txs/s not 20 txs/s. R18 added. |

## Local verification session 2026-10-25

- Phase 2 dynamic regime re-verified locally on multi-producer devnet: ordinary (3/3 included, 0-1ms latency) + burst (5/5 included, 0-1ms latency) + probe (3/3 included, 1-4ms latency).
- Phase 3 matrix re-verified locally on multi-producer devnet: low-steady-1 (3 seeds, 100% inclusion), low-burst-1 (3 seeds, 100% inclusion), low-steady-multi (3 seeds, 100% inclusion).
- Gap check redone from nummer13 (2026-10-25): sol-znn/go-zenon-fuzz (fuzzing, no overlap), 0x3639/testnet (infra tooling, no overlap), TminusZ/zenon-developer-commons (research papers, no empirical suite). Conclusion unchanged: lab fills a real gap.
- Suite runner verified: `run-suite.sh check` passes all R1-R20 regression assertions.
- Local verification files moved to `results/phase-{2,3}/local-verification/` to avoid glob collision with original unfused results.
- Crypto dependency inventory completed: `docs/crypto-dependencies.md` — full inventory of signature scheme (Ed25519), hash functions (SHA3-256/SHA-256/Keccak-256/RIPEMD-160), producer keys (BIP-39+SLIP-0010), bridge keys (secp256k1 TSS), address derivation, PoW, wallet encryption (Argon2id+AES-256-GCM), PTLC, P2P transport crypto.

## Genesis defect (found in phase 1, verified locally)

The committed `docker/devnet/genesis.json` at `e89f91f` fails `CheckPlasmaInfo`:
3 Plasma fusions totaling 3e12 raw QSR are declared, but no genesis block credits
the Plasma contract (`z1qxemdeddedxplasmaxxxxxxxxxxxxxxxxsctrp`) with that QSR.
`znnd` refuses to boot from the committed genesis. Fix: add the Plasma-contract
QSR block (3e12) and raise QSR `totalSupply` by 3e12. **Fix merged upstream as PR #201**
(merged 2026-10-21). The lab still uses the pinned commit with the lab-local patch.

## Key findings so far

1. **Fused plasma eliminates PoW entirely.** With 210K plasma (100 QSR), 5 sequential
   transactions complete in 0.02s with 0 PoW. Without plasma, the same 5 transactions
   take 41-79s (all in PoW computation).
2. **Single-account burst rejection is caused by PoW delay, not account-chain limits.**
   With plasma: 100% inclusion (0.05s). Without plasma: 20-40% inclusion (26-99s).
   The PoW delay (5-50s) creates a race window where all goroutines build on the
   same stale frontier.
3. **Each simple self-transfer consumes exactly 21000 plasma.** 100 QSR = 210000 max
   plasma. Plasma regenerates over time (rate TBD).
4. **No significant static vs dynamic difference for unfused accounts at low volume.**
   Dynamic Plasma pricing only activates when momentum-level plasma usage exceeds
   the target.
5. **No observable static vs dynamic difference for fused accounts at low volume.**
   Dynamic pricing (nextFusionPrice/workPrice=1000) does not affect existing fusion
   holders making simple transfers. The repricing threshold is never reached at
   our test volumes.
6. **Dynamic Plasma repricing fires at >50 fused txs/momentum.** Confirmed with
   200-tx bursts: nextFusionPrice rises from 1000 to 1100 (+10% cap). Price decays
   ~5% per empty momentum, returning to floor (1000) after 2 empty momenta.
   Sustained repricing requires sustained load; sporadic bursts create oscillation.
7. **The repricing formula is** `change = price * (used - target) / target / 20`,
   capped at ±10% per momentum. At max capacity (200 txs = 4x target), the raw
   change would be +150 but the cap limits it to +100 (1100).
8. **Under multi-sender contention, plasma availability determines inclusion.**
   Accounts with more plasma achieve higher inclusion rates when demand exceeds
   per-momentum capacity. No automatic PoW fallback — low-plasma transactions
   are rejected with "not enough TotalPlasma provided".
9. **Plasma is a per-momentum rate limit, not a permanent budget.** Each tx
   consumes exactly 21000 plasma temporarily. After confirmation, the full
   fused amount is available again. 100 QSR = 210,000 plasma = 10 simple
   txs per momentum. The two-counter system (committed/uncommitted) means
   plasma "depletion" is just unconfirmed backlog.
10. **PoW transactions have 100% inclusion when sent sequentially.** Each
    account sends 1 tx with PoW (difficulty 31.5M at workPrice=1000).
    Inclusion is near-instant (0-34ms). PoW computation dominates latency
    (3.3-75.8s, geometric distribution).
11. **workPrice repricing is impractical to trigger via PoW alone.** The
    PowPlasmaTarget is 50 txs/momentum. With PoW taking 3-76s per tx on
    typical hardware, reaching 50 PoW txs in a single momentum requires
    50+ parallel CPUs or GPU acceleration. On mainnet, workPrice repricing
    is effectively dormant.
12. **RPC latency is unchanged under transaction burst load.** Tested with
    100/200/500-tx bursts: p50 latency increases by only 0-1ms (9-10ms →
    10-11ms). No endpoint is disproportionately affected. The node is I/O-bound
    for RPC, not CPU-bound. Wallets and explorers should not experience RPC
    degradation during high-demand periods.
13. **The 200 tx/momentum limit is strictly enforced; excess is queued.**
    No momentum ever exceeds 200 txs. Excess transactions are queued in the
    mempool and included in subsequent momenta (100% inclusion with sufficient
    plasma). Queue drain rate: ~200 txs per 3 momenta (2 empty momenta between
    batches). Effective throughput under sustained saturation is ~67 txs/momentum,
    not 200/momentum. The only rejection cause under saturation is plasma
    exhaustion (rapid-fire sending beyond per-account plasma capacity).
14. **No plasma-based prioritization under saturation (same-price txs).** When
    all transactions pay the same price-per-base-plasma (floor price), the mempool
    does NOT prioritize fused (plasma) txs over unfused (PoW) txs. Selection is
    effectively random (hash-based tie-break). Fused txs are included first only
    because they arrive instantly (0s PoW), not because of any priority mechanism.
    Plasma is a rate limit, not a fee market at floor prices.
15. **Price-based prioritization EXISTS when prices differ.** The content
    selector sorts by price-per-base-plasma (dp.HigherPrice). Transactions that
    overpay (FusedPlasma > BasePlasma) are included before lower-priced txs
    under saturation. This creates a dormant fee market: priority can be bought
    by overpaying, but at floor prices all txs have equal priority.
16. **Transaction type plasma costs match source exactly.** Self-transfer=
    21,000, send+100B data=27,800 (21000+68×100), contract call=52,500
    (EmbeddedSimple). All 3 seeds identical. The go-zenon plasma cost model
    is empirically confirmed for all tested types. Data payload costs 68
    plasma/byte linearly. Contract calls cost 2.5× a simple transfer.
17. **The per-account mempool cap of 500 uncommitted blocks is real and
    strictly enforced.** When an account has 500 pending transactions, new
    submissions are rejected with "max uncommitted blocks per account
    reached". A drain/refill cycle was observed: after momenta include
    blocks, new txs are accepted again.
18. **Frontier conflict resolution follows the documented price-then-hash
    rule exactly.** Higher price-per-base-plasma wins; on tie, smallest hash
    wins. 15/15 correct predictions across 3 seeds. Overpaying guarantees
    winning a frontier conflict. Two distinct error messages reveal the
    mechanism: "block price is smaller" (price-based) and "hash tie-break
    is worse" (hash-based).
19. **Data payload plasma cost formula is exact: 21000 + 68*N bytes.**
    Confirmed for all sizes 0 through 16,384 bytes across 3 seeds. The 16KB
    limit (MaxDataLength) is strictly enforced — >16KB rejected with
    "account-block data field is too big". A max-size tx costs 1,135,112
    plasma (~541 QSR). The per-momentum plasma budget (4.2M) limits large-data
    txs to 3 per momentum; the 8MB wire size budget is not binding at 16KB.
    Inclusion latency is independent of data size (0-6ms).
20. **Contract block cap of 80/momentum is NOT the binding constraint for
    contract throughput.** The plasma budget (4.2M) limits user-initiated
    contract calls to ~73-75 per momentum (52,500 plasma each for
    EmbeddedSimple). The cap and plasma budget are mathematically aligned
    (4,200,000 / 52,500 = 80), making them redundant. Stake generates only
    1 contract block (ContractReceive, no ContractSend). Contract calls
    follow a 2-momentum pipeline: user sends in N, contract blocks in N+1.
    Effective sustained contract throughput is ~37-40 calls per 10s
    momentum-period, not 80.
23. **The transaction validation pipeline is robust against malformed inputs.** All 13 negative test cases (negative amount, oversized amount, zero ZTS with non-zero amount, non-zero FromBlockHash on send, invalid version, wrong chain ID, invalid block type, genesis receive type, contract receive for user address, receive with non-zero amount/ZTS/ToAddress) are correctly rejected with informative error messages matching the verifier source. Zero-amount sends are valid (boundary case). No validation gaps found.
22. **Momentum production rate scales as BlockTime × N/K** (N pillars, K active). On the lab devnet (3 pillars, 1 active): 30s mean, 3× slowdown. The distribution is irregular (10–180s, stddev 22.4s) and does NOT match a uniform shuffle model — the deterministic shuffle (math/rand with height seed) is biased toward consecutive slots for small permutation spaces. Momentum timestamps are slot start times (deterministic from genesis), not wall-clock times. Transaction load does not affect production rate. Effective throughput: ~6.7 txs/s (200 txs / 30s), not 20 txs/s.
21. **The 280 content cap is a correctness bound, not a throughput limit.**
    The actual maximum content per momentum is ~200 (plasma budget limit).
    Config A seed 1 achieved exactly 200 content entries (200 × 21,000 =
    4,200,000 = MaxBasePlasmaInMomentum). The 280 cap (200 user + 80
    contract) was never approached. Mixing contract calls reduces total
    content because Stake calls cost 52,500 plasma (2.5×) for 2 content
    entries vs. 21,000 for 1 (self-transfer). Self-transfers are 25% more
    plasma-efficient per content entry. The theoretical 280 maximum requires
    precise two-momentum orchestration (80 Stake in N-1, 200 self in N).

## Multi-producer setup (phase 23)

Three znnd instances run simultaneously for multi-producer tests:
- **Pillar 1** (dev1): `~/lab-devnet/`, RPC 127.0.0.1:57997, P2P 127.0.0.1:56995
- **Pillar 2** (dev2): `~/lab-devnet-pillar2/`, RPC 127.0.0.1:57996, P2P 127.0.0.1:56996
- **Pillar 3** (dev3): `~/lab-devnet-pillar3/`, RPC 127.0.0.1:57995, P2P 127.0.0.1:56997

All use `P2PBackend: "libp2p"` (forced, skipping spork oracle). BootstrapPeers
configured in a mesh. All three nodes share the same lab-patched genesis.

## Suite tooling

- `suite/cmd/labctl/main.go` — Go CLI: info, ordinary, burst, multiburst, probe,
  fuse, fund, fusefor, receive, waitfor, matrix, hvburst, sustain, contend,
  deplete, powlat, powburst, mixcontend, saturate, mixsaturate, overpay, regen,
  txtype, poolcap, frontconf, datasize, contractcap, contentmix, valedge.
  Disposable devnet keys only.
- `suite/rpcload.sh` — RPC latency under load measurement (phase 10).
- `suite/run-suite.sh` — one-command runner: phase2|phase3|phase4|phase6|phase7|phase8|phase9|phase11|phase12|phase13|phase15|phase16|phase17|phase18|phase19|phase20|phase21|phase22|phase24|check|all.
- `suite/json2csv.py` — exports all JSON results to summary.csv.
- `suite/check-regressions.py` — regression assertions R1-R20.
- `hvburst` — high-volume burst with per-momentum price tracking (phase 6).
- `sustain` — continuous multi-account burst for sustained repricing tests (phase 6).
- `deplete` — plasma depletion + regeneration test (phase 8). Fuses QSR, sends
  txs until exhaustion, waits for regeneration, tests PoW fallback.
- `powlat` — PoW latency measurement (phase 9). Sends 1 PoW tx per account
  from N unfused accounts, measures PoW time + inclusion latency.
- `powburst` — PoW repricing attempt (phase 9, impractical on this hardware).
  Sustained multi-account PoW burst; workPrice repricing not triggered.
- `mixcontend` — mixed fused/unfused contention (phase 11). Fused accounts
  get first-momentum inclusion; unfused accounts spread across momenta.
- `saturate` — saturation test (phase 12). N accounts with equal plasma send
  M txs each, exceeding 200 tx/momentum limit. Measures queue drain behavior.
- `mixsaturate` — mixed fused/unfused saturation (phase 13). N fused + N unfused
  accounts send txs simultaneously, exceeding 200 limit. Measures whether plasma
  affects mempool prioritization under saturation. Supports different tx counts
  for fused vs unfused groups.
- `overpay` — content selector price priority test (phase 14). N groups with
  different overpay multipliers (1x, 2x, 4x, 8x) send txs simultaneously,
  exceeding 200 limit. Measures whether higher-priced txs are included first.
- `regen` — plasma regeneration rate test (phase 15). Fuses QSR, depletes
  plasma via self-transfers, samples plasma at 1/3/6/10 momenta. Confirms
  plasma regenerates to full within 1 momentum.
- `txtype` — transaction type diversity test (phase 16). Fuses QSR, then tests
  self-transfer, send+data, cross-account send+receive, and contract call
  (stake). Records plasma cost per type and verifies against go-zenon source.
- `poolcap` — mempool cap test (phase 17). Fuses large QSR, sends rapid-fire
  self-transfers to hit the 500 uncommitted blocks per account cap. Records
  first reject index and error message.
- `frontconf` — frontier conflict resolution test (phase 17). Creates
  conflicting transactions at the same height with different plasma amounts.
  Tests both hash tie-break (equal plasma) and price priority (overpay).
  Verifies the price-then-hash replacement rule from go-zenon source.
- `datasize` — data payload size limits test (phase 18). Sends transactions
  at increasing data sizes (0 through 16KB+1), verifies the plasma cost
  formula 21000+68*N, tests the 16KB limit enforcement, and bursts max-size
  txs to observe per-momentum plasma budget limits.
- `contractcap` — contract block throughput cap test (phase 19). N accounts
  send Stake contract calls simultaneously to test the per-momentum contract
  block cap (80). Verifies that plasma budget, not contract cap, is the
  binding constraint. Optional base index parameter for multiple seeds.
- `contentmix` — momentum content capacity test (phase 20). N self-transfer
  accounts + M Stake accounts send transactions simultaneously. Measures
  actual max content entries per momentum. Verifies that plasma budget,
  not the 280 content cap, is the binding constraint. Optional base index
  parameter for multiple seeds.
- `valedge` — transaction validation edge cases test (phase 22). Sends 14 intentionally malformed transactions (negative amount, oversized amount, wrong version, wrong chain ID, invalid block type, etc.) and records exact error responses. Verifies the validation pipeline rejects all malformed inputs with informative error messages. Usage: `labctl valedge <idx> <qsr>`.
- `momentum-rate.py` — momentum production rate analysis (phase 21). Polls
  frontier momentum to measure inter-momentum time distribution. Compares
  with theoretical model from election algorithm. Usage: `python3 momentum-rate.py
  --duration SECS --out FILE`.
- `powlat` now accepts optional baseIdx (default 200). Usage: `labctl powlat <n> <amt> [baseIdx]`.
- `mixcontend` now accepts optional fusedBaseIdx, unfusedBaseIdx, skipfusion.
  Usage: `labctl mixcontend <f> <u> <txPerAcct> <amt> [fusedBaseIdx] [unfusedBaseIdx] [skipfusion]`.
- `momentum-rate-load.sh` — runs momentum-rate collector while sending a
  transaction burst to test if load affects production rate.

## Rules for updating
Only the owner (mr.stockholm.Z) edits this file. After each cloud result: store the report under
`results/phase-N/`, re-verify the key numbers locally where the machine allows, then bump `next_phase` only if the
phase's gate in PLAN.md is met; otherwise leave it and write the reason into the table.
