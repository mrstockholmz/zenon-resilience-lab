# Phase 29 — devnet checks of open go-zenon PRs (lab work item C, material for M12)

Status: #310 DONE (jail lifecycle complete, 7/7 testable checks PASS);
#306 A/B v1 done (honest negative) + drip-liar discriminating re-run IN PROGRESS:
A2 (post-fix victim) collecting since 10:22 CET, B2 (pre-fix) swap armed 10:55 CET
via scripts/swap-b2.sh; collect next lab session per STATE.md
Date: 2026-11-18 (session 1 00:15–02:15 CEST; session 2 09:15–11:00 CET)

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
- 23:44:02 node n3 (dev3) killed (epoch 1794962642). dev3 never produces again; network keeps producing
  on dev1+dev2 only.

Timeline continued (UTC, from `raw/monitor.jsonl` + `raw/auto-unjail.log`; the watcher
script writes local CET time with a mislabeled `Z` suffix — its lines anchor to the
file mtime, e.g. the last line "2026-11-18T01:55:18Z" ≡ epoch 1794966918 ≡ 01:55:18 CET):
- 23:44:02–00:54:48 kill→jail window: 95 monitor samples, dev3 in producers 0 times,
  producedMomentums constant at 24 — production stop is clean and the chain does not stall.
- 00:54:48 (epoch 1794966888, heights 599–604): the armed watcher `scripts/auto-unjail.py`
  detects dev3 IS JAILED. The pillar-contract Update runs every 300 momenta, so the jail
  executed at the h601/602 Update — the same cadence that at h301/302 correctly judged
  the 4-tick streak below the 12-tick threshold. Kill→jail latency: 4246 s ≈ 70.8 min
  ≈ 14 election ticks of 300 s (window requires ≥ 12 ticks spanning ≥ 3 missed slots —
  observed latency is consistent with the code constants).
- 00:55:18: unjail send (100 ZNN from the dev3 stake address, derived index 7) included
  by h604; total ZNN supply 120500000000000 → 120490000000000 (−100 ZNN exactly, burned);
  isJailed → false. The ~60 s jail window was shorter than the monitor's 45 s poll, so
  no monitor row carries isJailed=true — the jail evidence is the watcher log plus the
  supply drop between monitor samples h599 (supply unchanged) and h604 (supply dropped).
- 00:55:30-ish: watcher restarts n3; dev3 resumes producing at h612 (first post-unjail
  production sample ts 1794967012, 124 s after the jail detection). producedMomentums
  climbs 24 → 793+ over the following hours; dev1+dev2+dev3 all produce again.
- NOT OBSERVED live: rewards/votes while jailed (jail window ~60 s and dev3's node was
  down during it; slot-exclusion while jailed is indistinguishable from the dead-node
  baseline in this setup). Next-epoch jail recurrence needs a 24 h epoch — unreachable
  in a lab session.

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

`python3 check-phase29.py` final output (2026-11-18 ~09:45 CET):

```
PASS  310: dev3 stops producing after its node is killed
PASS  310: network keeps producing with 2 of 3 pillars (no stall)
PASS  310: pillar jailed after >=12-tick miss window (h601/602 Update; ~14 ticks after kill)
PASS  310: unjail accepted from stake address with exactly 100 ZNN
PASS  310: unjail burns exactly 100 ZNN (total supply drops)
PASS  310: unjailed pillar resumes producing after node restart (h612, 124 s after unjail)
PASS  310: unjail with 99 ZNN refused (amount gate)
NOT TESTED 310: rewards/votes while jailed (jail window ~60 s, node down — not observable)
NOT TESTED 310: jailed pillar in the next epoch (24h epoch, unreachable in-session)
PASS  306 post-fix: production continues while liar peer attached
FAIL  306 pre-fix control: sustained hold under the same liar (honest negative, see below)
PASS  306: go test -race ./protocol/... green at head dfbc164
9 PASS, 1 FAIL, 2 NOT TESTED
```

### PR #310 — verdict on the devnet lifecycle

Every testable claim of the PR held end to end on the 3-pillar devnet at head `bb1984fa`:
a stopped pillar is excluded from production without stalling the chain, the network
keeps producing at full height on the remaining pillars (no fallback election needed with
2/3 active), the jail lands at the first pillar-contract Update whose judgement window
covers ≥12 ticks of misses (~70 min after the stop on this 10 s / 300 s-tick devnet),
the 99 ZNN unjail is refused while exactly 100 ZNN from the stake address is accepted and
burned (supply −100 ZNN exactly), and the unjailed pillar resumes producing within ~2 min
of its node restart. What the devnet cannot reach: the 24 h next-epoch recurrence and
live reward/vote observation during a ~60 s jail window.

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

### PR #306 — drip-liar v6 re-run (2026-11-18 session 3)

The v6 liar (`liar-patch-v6.diff`) keeps download sessions open ~40 s with delayed
ancestor-search replies. Findings from collecting the armed A2/B2 run:

- **B2 (the 10:55 pre-fix swap) was an invalid control**: the victim restarted on
the pre-fix binary but the liar never re-peered. The liar kept a zombie peer entry
from the killed post-fix victim process and reset the new victim's eth/61 stream
(`handshake failed`, victim log 10:55:26); the victim then ran 8.5 h with zero
sessions (solo baseline 2.002 mom/min, `peers=0` in every logger row). Fix: liar
restarted 19:19 CET (B2b).
- **Setup discovery**: victim (dev1) and liar (dev2) each produced their own
canonical chain from h67 — every mutual sync needs a >30-momentum rollback and is
refused by the cap. In B2b the victim logged, per session (all within one second):
`Synchronizing` → `looking for common ancestor` → `Synchronization completed` →
`Inserting side-chain` → `Block import failed … can't rollback to {… h66}. Too
far` — 126 such no-op sessions in 31.5 min (one every ~15 s), each importing
nothing. This is exactly issue #291/#306's session shape, against the pre-fix
binary.
- **Result**: the pre-fix victim still produced at 1.968 mom/min (63 momenta,
gap median 20 s, max 110 s) — 98.4% of its own solo baseline. The hold did NOT
manifest, because the rollback-cap fast-fail ends every session within the same
second; a hold needs a session that stays open across producer ticks.
- **Post-fix comparison**: A2 (post-fix, same liar) produced 2.022 mom/min with
the liar attached; across all post-fix phases production never stalled.

Verdict: second honest negative on the pre-fix regression, with the mechanism now
pinned (session lifetime, not session existence, gates the hold). Positive support
for the PR stands: no post-fix stall in any phase, race tests green.

Ground truth: `raw/drip-window-stats.json` (extractor `scripts/drip-stats.py`),
`raw/liar-test.jsonl`, `raw/victim-zenon-20261118-part1.log.gz`,
`raw/{victim,liar}-b2b-keylines.log`, `raw/swap-b2b.log` (below).

Final checker output (2026-11-18 ~20:00 CET): 11 PASS / 2 FAIL / 2 NOT TESTED
(exit 1 by design — both FAILs are the honest-negative pre-fix controls).

## Follow-ups (next lab session)

1. ~~Collect the #310 jail lifecycle~~ — DONE (see above).
2. ~~#306 drip A/B (v6 liar)~~ — DONE (session 3, see “Drip-liar v6 re-run”
   below): B2 (10:55 swap) was an invalid control (liar never re-peered —
   zombie peer entry reset the new victim's eth/61 handshake until the liar
   itself was restarted 19:19 CET); B2b re-run produced 126 no-op sessions in
   31.5 min against the pre-fix victim — production still 1.968 mom/min
   (98.4% of solo baseline). check-phase29.py extended, devnet torn down.
3. Optionally: next-epoch jail-recurrence check needs a 24 h run — schedule via
   STATE.md long-run listing if desired.
4. A discriminating pre-fix hold repro needs a liar whose session STAYS OPEN
   across producer ticks without tripping the rollback-cap fast-fail (e.g. a
   fork within the last 30 momenta that is never served). Both liar designs so
   far (never-serve-blocks v1; deep-fork instant-fail v6) end sessions too
   quickly for the pre-fix hold to manifest.
