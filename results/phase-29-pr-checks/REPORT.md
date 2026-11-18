# Phase 29 — devnet checks of open go-zenon PRs (lab work item C, material for M12)

Status: IN PROGRESS (jail lifecycle completes unattended tonight via scripts/auto-unjail.py;
306 liar test DONE — control negative, reported honestly)
Date: 2026-11-18 (session 00:15–02:15 CEST; wrap-up ~02:15)

## What was tested

### PR #310 — `feat: jail inactive pillars and add a paid unjail` (head `bb1984fa`)

Binary: built from `~/p29-310` (worktree at `bb1984fa854dbf0bf42b69f90f5feb86c30897de`).
Devnet: fresh 3-pillar chain-70 network (`raw/genesis-p29.json`), the lab's deterministic
3-pillar genesis with `PillarJailSpork` (id `...0008`) **pre-activated at genesis**
(genesis `SporkConfig`, `activated: true, enforcementHeight: 0` — the same mechanism the
node uses for the az/htlc genesis sporks). Nodes `~/lab-devnet-p29/n{1,2,3}` (dev1/dev2/dev3
producers), RPC 47971-3, `Restart` none (plain processes, `nice -n 10`).

The jail constants are wall-clock: one election tick = `BlockTime*NodeCount` = 10 s * 30 =
300 s; `PillarJailWindowPeriods` = 12 ticks -> a pillar that stops producing is jailed
earliest ~60 min later. Epoch duration (24 h, static) could not be reached in-session.

Timeline (UTC):
- 23:38 genesis, nodes boot; all three pillars produce at 1.0x (10 s momenta).
- 23:41 derived-account map resolved (`raw/derived-addrs.txt`): dev3 stake = index 7.
- 23:42 dev3 stake funded (300 ZNN + 600 QSR from derived 3) and fused 300 QSR.
- 23:43 negative check: unjail with 99 ZNN refused by the node (`invalid token or amount`).
- 23:44:02 node n3 (dev3) killed. dev3 never produces again; network keeps producing
  on dev1+dev2 only.
- (jail expected when the finished-tick window covers the first missed tick + 12 ticks,
  ≈ 01:42-01:50 CEST; monitor: `scripts/monitor-p29.py` → `raw/monitor.jsonl`)
- UPDATE 01:05 CEST: the first pillar-contract Update at h301/302 judged only ticks 0..7
  (Update runs every `UpdateMinNumMomentums` = 300 momenta) — streak 4 ticks, below the
  12-tick jail threshold, correctly NOT jailed. Next judgement at h602 (~02:35 CEST):
  ticks 8..~21 with streak from tick 4 → 18 ≥ 12 → jail. That is past the session kill,
  so `scripts/auto-unjail.py` (nohup, PID in STATE.md) was armed: it polls for
  isJailed=true, snapshots ZNN supply before/after, runs `p29probe unjail 7 10000000000 dev3`
  (100 ZNN from the dev3 stake address, index 7), restarts n3, and logs every step to
  `raw/auto-unjail.log`. Next lab session collects the results.

Checks: `python3 check-phase29.py` prints PASS/FAIL/NOT TESTED from the raw evidence.

### PR #306 — `fix(protocol): sessions that import nothing no longer hold production` (head `dfbc164`)

- Build: `go build ./cmd/znnd` from `~/review/pr-306` (head `dfbc1644b0ab…`; note `./app`
  is a library, the node main is `./cmd/znnd`).
- Unit/integration evidence at head: `go test -race -count=1 ./protocol/...` green
  (protocol 35.5 s, downloader 11.2 s, fetcher 25.1 s) — includes the new readiness,
  handoff and scripted-origin tests from the PR itself (`raw/pr306-protocol-race-tests.log`).
- Devnet A/B (liar peer, reproducing issue #291 end to end):
  - Liar build `~/p29-liar` = `dfbc164` + a LAB-ONLY patch (env `ZNN_LIAR`, never for
    production): handshake advertises TD +66,000,000; `GetBlocksMsg` requests are swallowed
    without a reply; the syncer never starts client sessions (keeps its own chain view).
  - Chain-71 devnet: victim = solo-pillar node (dev1) running the **post-fix** build;
    f2 = a second pillar (dev2) that first syncs the canonical chain, then restarts in
    liar mode and forks alone at the canonical tip. Handshake advertises a TD inflated
    by +66,000,000 (the victim then reports `targetHeight` 66000000 while chasing it); sessions find the fork point, queue fork hashes, and stall on the
    swallowed block fetch — importing nothing, exactly issue #291's shape
    ("blocks sit in the queue behind a head that is never served").
  - Phase A (post-fix victim): production must continue past the 60 s stall window.
  - Phase B (control): the victim's binary is swapped to the pre-fix build
    (`8999aba^` = `65240b1`, same data dir); the same liar must hold production.
  - Logger: `scripts/liar-logger.py` → `raw/liar-test.jsonl` (phase labels from
    `raw/liar-phase.txt`).

## Results

`python3 check-phase29.py` output at wrap-up (02:15 CEST):

```
PASS  310: dev3 stops producing after its node is killed
PASS  310: network keeps producing with 2 of 3 pillars (no stall)
NOT TESTED 310: pillar jailed after >=12-tick miss window  (pending, ~02:35 CEST)
NOT TESTED 310: unjail accepted from stake address with exactly 100 ZNN
NOT TESTED 310: unjailed pillar resumes producing after node restart
PASS  310: unjail with 99 ZNN refused (amount gate)
NOT TESTED 310: jailed pillar in the next epoch (24h epoch, unreachable in-session)
PASS  306 post-fix: production continues while liar peer attached
FAIL  306 pre-fix control: sustained hold under the same liar
PASS  306: go test -race ./protocol/... green at head dfbc164
5 PASS, 1 FAIL, 4 NOT TESTED
```

### PR #306 — honest verdict on the devnet control

The FAIL on the pre-fix control is a **negative result, reported as such** (G-rules):
the liar repro did NOT reproduce a sustained production hold in either binary.

Ground truth is the victim's momentum-inserted wall-clock log (20 s jsonl polls were
too coarse to see the structure):

| phase | binary | span | rate | victim-log production gaps |
|---|---|---|---|---|
| A | post-fix `dfbc164` | h2–80 | ~0.36x (tail avg 27.6 s/momentum) | 70, 120, 70, 80, 80, **160**, 80, 70 s |
| B | pre-fix `65240b1` | h81–151 | ~0.33x | 70, **140**, 70, 100, 80, 80, 120 s |

The gap distributions overlap almost completely (max 160 s post-fix vs 140 s pre-fix).
Root cause of the non-discrimination, read from `raw/liar-test.jsonl`: against this
liar the victim's sync sessions **end quickly** (`syncState` 1→2, target 0) because
the liar serves hash packs but no blocks — the victim marks sync done, the session
ends, and production squeezes through between attempts in BOTH binaries. Issue #291's
sustained-hold shape needs a session that never finishes; our never-serve-blocks liar
(`GetBlocksMsg` swallowed) gets abandoned before it can hold.

What the phase DOES support:
- post-fix production is never squeezed to a stop (0.27–0.36x throughout, no stall) — PASS.
- `go test -race ./protocol/...` green at head, including the PR's own readiness/handoff
  tests — the fix's semantics are unit-tested.
- bonus phase-28 confirmation: both #306 builds halt at h1 on a genesis that
  pre-activates the pillar-jail spork (they predate the spork whitelist); chain 71's
  genesis was rewritten without it.

What it does NOT support: an independent devnet confirmation that pre-fix holds
production where post-fix does not. A discriminating repro needs a liar whose sync
session stays open (e.g. serve the fork head hash, then answer block requests with
ever-deeper hashes so the queue never drains and the session never completes) —
noted as follow-up in the project file.

## Follow-ups (next lab session)

1. Collect `raw/monitor.jsonl` + `raw/auto-unjail.log`: jail event (~02:35 CEST),
   unjail burn (supply delta must be exactly 100 ZNN), n3 restart, dev3 resuming.
2. Re-run `check-phase29.py` — the remaining NOT TESTED rows should flip.
3. Optionally: next-epoch jail-recurrence check needs a 24 h run — schedule via
   STATE.md long-run listing if desired.
4. #306: build a session-staying-open liar for a discriminating A/B re-run.
