# State

<!-- machine-readable: the cloud brief and the head read this block -->
```
next_phase: 6
cloud_sessions_run: 1
local_sessions_run: 3
pinned_go_zenon: e89f91f68b922a65370b7ffd9cfe860045a99b29
pinned_sdk: n/a (suite is Go, imports go-zenon module directly at pinned_go_zenon; no separate SDK dependency)
plasma_mode: static<30 / dynamic>=30 (devnet genesis spork 0000...0002, enforcementHeight 30)
updated: 2026-10-22
```

## Phase log
| phase | status | result file | note |
|---|---|---|---|
| 1 gap check + baseline | partial-pass | results/phase-1/20261020-cloud-phase1-baseline.md, results/phase-1/20261021-gap-check.md | Baseline proven (round trip, PoW fallback, plasma regime pinned). Gap check redone locally 2026-10-21: no existing suite measures plasma/demand/latency — lab fills a real gap. Genesis defect found; fix merged upstream as PR #201. |
| 2 smallest experiment | done | results/phase-2/20261021-phase2-static-results.md, results/phase-2/20261021-phase2-dynamic-results.md | Ordinary + burst workloads in static (<30) and dynamic (>=30) regimes. Key finding: fused plasma eliminates PoW and enables instant inclusion; single-account burst rejection is caused by PoW delay, not account-chain limits. |
| 3 resource matrix | done | results/phase-3/20261021-phase3-matrix-results.md | Low vs high fused x steady vs burst x 1 vs multi accounts, 3 seeds each. 100% inclusion for sequential (all regimes) and for fused burst. 20-40% inclusion for unfused burst (PoW race). |
| 4 comparison | done | results/phase-4/20261022-phase4-comparison-results.md | Fused plasma static vs dynamic: no observable difference for existing fusion holders at low volume. Dynamic pricing (nextFusionPrice/workPrice=1000) only affects new fusions and PoW fallback under high demand. |
| 5 package | done | suite/ | One-command runner (suite/run-suite.sh with phase2/phase3/phase4/check/all), labctl tool, raw JSON results, CSV export (json2csv.py), regression assertions (check-regressions.py R1-R5 all passing). Documentation: suite/README.md with full reproduction instructions. Main README updated. |
| 6 next experiment | proposed | docs/phase-6-next.md | High-volume burst to trigger Dynamic Plasma repricing. Decision informed: when to activate dynamic-plasma spork on mainnet. |

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

## Suite tooling

- `suite/cmd/labctl/main.go` — Go CLI: info, ordinary, burst, multiburst, probe,
  fuse, fund, fusefor, receive, waitfor, matrix. Disposable devnet keys only.
- `suite/run-suite.sh` — one-command runner: phase2|phase3|phase4|check|all.
- `suite/json2csv.py` — exports all JSON results to summary.csv.
- `suite/check-regressions.py` — regression assertions R1-R5.

## Rules for updating
Only the owner (mr.stockholm.Z) edits this file. After each cloud result: store the report under
`results/phase-N/`, re-verify the key numbers locally where the machine allows, then bump `next_phase` only if the
phase's gate in PLAN.md is met; otherwise leave it and write the reason into the table.
