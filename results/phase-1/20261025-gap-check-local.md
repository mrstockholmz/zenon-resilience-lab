# Gap Check Redo — 2026-10-25 (local, from nummer13)

## Method
Re-checked the three adjacent projects from the lab machine, all reachable.

## Findings

### sol-znn/go-zenon-fuzz
- **Purpose:** Fuzzing go-zenon for crash bugs (decode, consensus, RPC saturation, soak).
- **Method:** In-memory fuzz targets + remote fuzzing against live nodes.
- **Overlap with lab:** None. Fuzzing looks for crash-triggering inputs; the lab measures
  plasma allocation, demand patterns, and confirmation latency under controlled workloads.
- **Last updated:** 2026-10-09 (FINDINGS.md).
- **Conclusion:** Complementary, not competing.

### 0x3639/testnet
- **Purpose:** Web tooling for coordinating a Zenon testnet (genesis builder, operator packages).
- **Method:** Node/Express + React/Vite web app.
- **Overlap with lab:** None. Infrastructure coordination, not empirical measurement.
- **Last updated:** 2026-09-24.
- **Conclusion:** Different problem domain.

### TminusZ/zenon-developer-commons
- **Purpose:** Research papers and proposals (architecture, feasibility, specs).
- **Relevant work:** `docs/research/transaction-admission-control.md` — a theoretical
  control-system model for Zenon admission (plasma + micro-PoW + global tuning).
  Status: exploratory, not an implementation claim.
- **Overlap with lab:** Conceptual overlap (plasma as admission control), but no empirical
  test suite, no devnet measurements, no latency data.
- **Conclusion:** The lab fills the empirical gap that the research papers identify but do not measure.

## Conclusion (unchanged)
No existing project measures plasma allocation, demand patterns, and confirmation latency
on a controlled devnet. The lab fills a real gap. The gap check from 2026-10-21 stands.
