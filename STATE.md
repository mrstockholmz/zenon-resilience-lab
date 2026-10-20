# State

<!-- machine-readable: the cloud brief and the head read this block -->
```
next_phase: 2
cloud_sessions_run: 1
pinned_go_zenon: e89f91f68b922a65370b7ffd9cfe860045a99b29
pinned_sdk: e89f91f68b922a65370b7ffd9cfe860045a99b29
plasma_mode: static<30 / dynamic>=30 (devnet genesis spork 0000...0002, enforcementHeight 30)
updated: 2026-10-20
```

## Phase log
| phase | status | result file | note |
|---|---|---|---|
| 1 gap check + baseline | partial-pass | results/phase-1/20261020-cloud-phase1-baseline.md | Baseline proven (round trip, PoW fallback, plasma regime pinned). Gap check blocked: external repos unreachable from cloud session. Genesis defect found (see below). |
| 2 smallest experiment | open | | Plan: run static (<30) and dynamic (>=30) sub-matrices. |
| 3 resource matrix | open | | |
| 4 comparison | open | | |
| 5 package | open | | |
| 6 next experiment | open | | |

## Genesis defect (found in phase 1, verified locally)

The committed `docker/devnet/genesis.json` at `e89f91f` fails `CheckPlasmaInfo`:
3 Plasma fusions totaling 3e12 raw QSR are declared, but no genesis block credits
the Plasma contract (`z1qxemdeddedxplasmaxxxxxxxxxxxxxxxxsctrp`) with that QSR.
`znnd` refuses to boot from the committed genesis. Fix: add the Plasma-contract
QSR block (3e12) and raise QSR `totalSupply` by 3e12. Local-only, not remote-reachable.
Disposition: candidate for minimal upstream fix PR.

## Rules for updating
Only the owner (mr.stockholm.Z) edits this file. After each cloud result: store the report under
`results/phase-N/`, re-verify the key numbers locally where the machine allows, then bump `next_phase` only if the
phase's gate in PLAN.md is met; otherwise leave it and write the reason into the table.
