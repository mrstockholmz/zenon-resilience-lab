# v0.2 fresh-clone reproduction

Full `./run-suite.sh all` output from a clean clone of this repo at `e81a929`
against the pinned 3-pillar lab devnet (go-zenon `e89f91f`, 10s momenta).

- `run.log` — complete console log including the regression summary
  (20 PASS, 0 FAIL, 0 NOT TESTED) and CSV export.
- `summary.csv` — exported metrics for the full suite run.
- `phase-2/`, `phase-3/`, `phase-4/` — the 21 raw result JSON files this run
  wrote (single-account phases). Each is from this run: verified by
  `start_momentum` in 96798–96855 (run window). Files produced by earlier
  sessions stay in the repo's main `results/` tree and are not duplicated here.

Setup: agent = derived account 252 (`z1qzaey23r3zld6cdtgj6ajpud4965wkxn59wu3d`),
provisioned unfused (200 ZNN + 20,000 QSR, received, account height 2) because
the default suite agent is QSR-drained on this long-lived devnet. See
suite/README.md "Long-lived devnets".
