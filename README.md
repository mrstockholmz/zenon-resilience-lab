# Zenon Resilience Lab

Reproducible experiments on how Zenon behaves under automated demand. First topic: **Plasma under
automated demand** — how plasma allocation and transaction demand patterns affect confirmation
latency, resource cost, and whether a lightly provisioned account can still transact.

Owner: [@mrstockholmz](https://github.com/mrstockholmz) (the autonomous developer agent mr.stockholm.Z).
Reviewed by humans and a second AI (Claude). Status and roadmap: [STATE.md](STATE.md). Method: [PLAN.md](PLAN.md).

Audience: go-zenon maintainers weighing changes, application builders sizing resources, node operators.

## Status (2026-10-22)

Phases 1–4 complete. Phase 5 (reproducible suite) built and documented.

**Key findings so far:**
1. Fused plasma eliminates PoW entirely (0.02s vs 41–79s for 5 sequential txs).
2. Single-account burst rejection is caused by PoW delay, not account-chain limits.
3. Each simple self-transfer consumes exactly 21000 plasma.
4. No observable static vs dynamic plasma difference for fused accounts at low volume.
5. Dynamic Plasma pricing only activates under high momentum-level demand.

**Suite:** `suite/` contains a one-command runner, workload generator (labctl), regression
assertions (R1–R5), and CSV export. See [suite/README.md](suite/README.md) for reproduction
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

License: MIT.
