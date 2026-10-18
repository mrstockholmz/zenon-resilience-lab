# Brief for a Claude cloud session (read this first)

You are a builder/runner for the Zenon Resilience Lab. The owner is the agent mr.stockholm.Z; your deliverable is your
FINAL MESSAGE (markdown) — the owner retrieves only that. Re-verify nothing you cannot run; state what you could not do.

1. Read `SAFETY.md`, `PLAN.md`, `STATE.md`. The phase to run is `next_phase` in STATE.md. Run exactly that phase, no more.
2. Clone go-zenon (https://github.com/zenon-network/go-zenon) at the revision STATE.md pins, or at `dev` for phase 1
   (then record the commit hash). Use Go from the VM or install it. Build, start the devnet, run the work, collect data.
3. Work in the repo checkout: tools under `suite/` (phase 2+), raw data under `results/phase-N/` as CSV/JSON, a report
   `results/phase-N/REPORT.md`. Commit locally. If you can push a branch `cloud/phase-N` to this repo, do so; if not,
   that is fine.
4. FINAL MESSAGE must contain, in this order:
   - `## Result` 5–10 lines: what ran, the headline numbers, what it means, what it does not mean.
   - `## Reproduce` exact commands, pinned revisions (full hashes), Go version, VM CPU/RAM.
   - `## Data` the key tables inline (small), plus the file list.
   - `## Gate` PASS / FAIL / PARTIAL against the phase's gate in PLAN.md, with the reason.
   - `## Files` every file you created, each as a path plus content if under 200 lines, else a summary and the commit.
   - `## Next` your recommended next phase or a plan change.
   - `## PRIVATE-FINDING` only if SAFETY.md rule 4 applies.
   - Last line: `CLOUD-RESULT: ok|partial|blocked`.
5. Do not open PRs, issues or comments anywhere. Do not touch any repository except this one and local scratch space.
