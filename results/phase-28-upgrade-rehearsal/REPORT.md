# Phase 28 — Upgrade-day rehearsal: unknown spork halt, restart loop, recovery

Date: 2026-11-17 (22:37–23:20 local). Author: mr.stockholm.Z (deep lane).
Machine: nummer13. All times local (CET) unless marked UTC.

## Purpose
Rehearse what happens to an operator running **official go-zenon v0.0.10**
(source `c3e0e88`, built at `/home/mrstockholmez/p28-go-zenon/build/znnd`,
sha256 prefix `f8bebc49312250c8`) when a spork it does not implement is
**activated** — the exact situation of the renewal spork `2be489e3…761f`
("v0.0.10", created on mainnet 2026-11-15, activation pending as of this
date). Test: the halt itself, the systemd restart loop under
`znnd.service` settings (`Restart=on-failure`, `RestartSec=15`), recovery
with a binary that knows the spork, and the warning time.

## Mechanism (verified in code, v0.0.10 c3e0e88)
- `chain/momentum_pool.go` `GotAllActiveSporksImplemented`: after every
  momentum insert, every defined spork with `Activated && EnforcementHeight
  <= frontier` must be in `types.ImplementedSporksMap`
  (`common/types/spork.go`; v0.0.10 knows only `az`, `htlc`, `bridge-liq`).
- On violation: log + console message, then `os.Exit(2)`. **Clean exit,
  code 2 — not a crash.**
- `vm/embedded/implementation/spork.go`: `ActivateSpork` sets
  `Activated=true` and `EnforcementHeight = frontier + SporkMinHeightDelay`
  (`vm/constants/embedded.go:80`, **delay = 6 momenta**).
- The check runs **on insert only** — not at node startup. A restarted node
  boots fine ("Initialized NoM") and dies on its first inserted momentum.
- Non-pillar nodes run the identical insert path; they die when they
  *receive* the first momentum at/after enforcement height.

## Setup
- Solo-pillar chain-69 devnet from the lab genesis **with the two genesis
  sporks v0.0.10 does not know removed** (`libp2p` id 0x…01,
  enforcement 20; `dynamic-plasma` id 0x…02, enforcement 30) so the
  rehearsal isolates the renewal-spork scenario: every enforcing spork on
  the chain is known to v0.0.10, plus one unknown spork created later.
  Side finding: stock lab genesis + official v0.0.10 would halt at h20
  regardless of any new spork (consistent with phase 27c).
- Data dir `~/p28-devnet` (RPC 127.0.0.1:57997), systemd unit
  `/etc/systemd/system/znn-p28.service` — a verbatim copy of
  `/etc/systemd/system/znnd.service` restart semantics
  (`Restart=on-failure`, `RestartSec=15`, `LimitNOFILE=65536`, `Nice=10`).
- Rehearsal spork: created by genesis SporkAddress via
  `results/phase-27-next-sporks/probe` (rebuilt against the c3e0e88 tree):
  name `p28-upgrade-rehearsal`, id
  `5191cc3ed277518471b709b7d24f1608bf9130d24a22c4cbb06a24535fcb7127`.
- Recovery binary: same c3e0e88 tree + 3-line patch adding the spork id to
  `ImplementedSporksMap` (branch `recovery/knows-p28-spork`), built as
  `build/znnd-recovery` (sha256 prefix `f93e6ad55b0a66bf`).

## Timeline (local)
| time | event |
|---|---|
| 23:02:38 | `ActivateSpork` send included (account height 2) |
| 23:03:10 | activation receive in momentum 9 (Txs:1); `enforcementHeight=15` set (frontier 9 + 6) |
| 23:03:18 | RPC-visible `activated=True, eh=15`, frontier 9 |
| 23:06:00 | node exits `status=2/INVALIDARGUMENT` inserting **height 15** — first halt |
| 23:06:15–23:09:18 | systemd restart loop: 14 × exit(2), one cycle per 15 s, each cycle the node boots ("Initialized NoM. Height: 15") and dies within 1–3 s producing h16 |
| 23:09:20 | recovery: `systemctl stop`, ExecStart → `znnd-recovery`, `systemctl start` |
| 23:09:30 | recovery binary inserts height 16; 17/18/19/20 follow; service stays `active` |

## Measurements
- **Warning time**: activation receive → halt = 6 momenta. On this slow
  devnet ~2 min 50 s (momentum pace 10–60 s). On mainnet's ~10 s cadence:
  **≈ 60 s from the activation momentum until every non-upgraded node
  halts** (pillar nodes halt producing the enforcement-height momentum
  themselves; other nodes halt on receiving it — same check, so ≤ 1
  momentum later).
- **Restart loop**: `Restart=on-failure` **restarts the node** (exit code 2
  is a failure) → infinite crash loop, one cycle per `RestartSec` (15 s).
  The node never reaches steady `active`; each cycle prints
  `Detected an unimplemented spork … Please upgrade your znnd binary …
  znnd is terminating`. journalctl-only diagnosis; nothing alerts.
- **"Congratulations" message never appears** in the halt path: at the
  enforcement-height insert the `unimplemented` branch fires first; the
  `justNow` congratulations print is unreachable when the node itself does
  not implement the spork.
- **Downtime**: first halt → first momentum under the new binary = 3 min
  30 s in the rehearsal, of which ~3 min was deliberate loop observation.
  Technical floor: stop → swap binary → start → producing within ~60 s
  (boot + one momentum).
- **Recovery**: a binary that knows the spork syncs past enforcement with
  no extra steps — no resync, no reindex; it just continues (h16+).
  Data dir untouched by the whole event (the halting insert had already
  persisted h15).

## Raw evidence
- `systemd-loop.log` — every start/exit/terminating line of the unit
  (88 lines).
- `momentums-tail.log` — last inserted momentums incl. h16–20 on the
  recovery binary.
- Journal source: `journalctl -u znn-p28` (unit now stopped+disabled;
  restart with `sudo systemctl enable --now znn-p28`).

## Not covered (stated honestly)
- Multi-node reception lag (non-pillar node measured via code path only,
  not run).
- Real mainnet momentum cadence; devnet pace is slower and irregular.
- Behavior under `Restart=no` (operators with custom units): node stays
  down after the single exit — strictly better for log clarity.
- Pillar-only production stop: with a jailing/fallback design the chain
  keeps producing while non-upgraded nodes halt — out of scope here.
