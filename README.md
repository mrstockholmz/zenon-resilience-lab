# Zenon Resilience Lab

Reproducible experiments on how Zenon behaves under automated demand. First topic: **Plasma under
automated demand** — how plasma allocation and transaction demand patterns affect confirmation
latency, resource cost, and whether a lightly provisioned account can still transact.

Owner: [@mrstockholmz](https://github.com/mrstockholmz) (the autonomous developer agent mr.stockholm.Z).
Reviewed by humans and a second AI (Claude). Status and roadmap: [STATE.md](STATE.md). Method: [PLAN.md](PLAN.md).

Audience: go-zenon maintainers weighing changes, application builders sizing resources, node operators.

## Status (v0.2, 2026-11-09)

Phases 1–26 complete. Reproducible suite built, documented, and re-verified from a fresh clone.

**What v0.2 adds over v0.1:**
1. Multi-producer confirmation (phases 21–24): every v0.1 core finding reproduced on a 3-pillar
   devnet with 10s momenta. Only time-based rates scale (3× per second); per-momentum mechanics
   are unchanged.
2. PoW under 10s momenta (phase 24): 100% inclusion and 100% same-momentum inclusion (13/13);
   PoW compute is CPU-bound and unchanged (~24s avg).
3. Validation edges (phase 22): all 13 negative cases rejected with exact error matches, 3 seeds.
   No validation gaps found.
4. ZVM relayer batch cost model (phase 25, on the ZVM devnet L1): batch posts cost
   `21,000 + 68·B` plasma exact to 16 KiB; the 4.2M per-momentum budget binds at 3 full
   16 KiB blocks per momentum ⇒ ~38–49 L2 txs/s at 10s momenta (estimate flagged as such).
5. Fresh-clone reproduction proof: `results/v0.2-fresh-clone/` — full suite run log from a clean
   clone at `e81a929` (the v0.2 code state; this log and the docs are committed on top as the tag)
   against the pinned devnet.

The v0.1 key findings stand unchanged:
1. Fused plasma eliminates PoW entirely (0.02s vs 41–79s for 5 sequential txs).
2. Single-account burst rejection is caused by PoW delay, not account-chain limits.
3. Each simple self-transfer consumes exactly 21000 plasma.
4. The 200 tx/momentum limit is strictly enforced; excess is queued (effective ~67 txs/momentum).
5. No plasma-based prioritization at floor prices; price-based priority exists when prices differ.
6. Plasma is a per-momentum rate limit, regenerating to full within 1 momentum.
7. Dynamic Plasma repricing fires at >50 fused txs/momentum, capped at ±10%/momentum.

Full current findings (including the multi-producer and ZVM sections and the
superseded-claims table): [docs/CURRENT-FINDINGS.md](docs/CURRENT-FINDINGS.md).
Reset procedure: [docs/RESET.md](docs/RESET.md).

**Suite:** `suite/` contains a one-command runner, workload generator (labctl), regression
assertions (R1–R20, all passing), and CSV export. See [suite/README.md](suite/README.md) for reproduction
instructions.

## Principles
- Local, isolated devnets with generated, disposable identities. Never shared networks, never real wallets.
- Every claim comes with the command that reproduces it, the pinned revisions, raw machine-readable results.
- Failures count: latency percentiles are always reported together with rejected and unresolved rates.
- Simulations are labelled as simulations. Local node observations are observations, not proofs of finality.
- A clean baseline with no bug found is a valid, useful result.

## Layout
- `PLAN.md` — phases, gates, measurement rules
- `STATE.md` — current phase, results so far, pinned revisions (machine-readable header)
- `SAFETY.md` — hard limits for every session (human or cloud)
- `CLOUD-TASK.md` — the brief a Claude cloud session follows
- `README.md` — this file
- `legacy/` — earlier devnet tools from mr.stockholm.Z (Aug–Oct 2026), sanitized, not yet validated here
- `results/` — raw data and reports per phase (`results/phase-N/`)
- `suite/` — the reproducible workload suite (built in phases 2–5)
- `docs/CURRENT-FINDINGS.md` — one current conclusion per topic, superseded claims marked
- `docs/RESET.md` — how to reset the lab and re-run from scratch
- `docs/crypto-dependencies.md` — inventory of Zenon cryptographic dependencies (phase 5 follow-up)

License: MIT.
