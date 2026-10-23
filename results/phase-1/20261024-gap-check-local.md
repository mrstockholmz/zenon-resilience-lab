# Gap Check — Redone Locally 2026-10-24

**Purpose:** Re-verify from this machine (nummer13) that no existing suite covers
what the Zenon Resilience Lab measures, now that sol-znn/go-zenon-fuzz and
0x3639/testnet are directly reachable.

## sol-znn/go-zenon-fuzz (last updated 2026-10-10)

**What it is:** Adversarial fuzzing of go-zenon `release/0.0.9` pinned as a submodule.
Coverage-guided, in-process fuzzers at parsers, wire/JSON-RPC entry points, and the
stateful block-apply pipeline.

**Fuzz targets:** abi, chain (pool race), consensus, decode (protobuf), remote
(RPC dispatch, RPC fuzz, budget), types.

**Findings:** 4 confirmed crashers, all LOCAL reach (not reachable from the network).
Focus is on robustness against malformed input, not performance or economics.

**Overlap with lab:** None. go-zenon-fuzz tests "can bad input crash the node?"
The lab tests "how do plasma allocation and demand affect confirmation latency,
resource cost, and transaction inclusion?" Different questions, different methods.

## 0x3639/testnet (last updated 2026-09-24)

**What it is:** Web tooling for coordinating a Zenon testnet. Admin creates operator
logins, collects pillar/seed-node registrations, generates genesis.json and config.json,
publishes them for wget access. Node/Express backend, React frontend.

**Overlap with lab:** None. This is infrastructure for running a shared testnet,
not a measurement suite. It does not generate workloads, measure latency, or test
plasma behavior.

## TminusZ/zenon-developer-commons

 plasma papers referenced in the original gap check (2026-10-21) — theoretical
 analysis, not empirical measurement. No code suite to run.

## Conclusion (unchanged from 2026-10-21)

No existing suite measures plasma/demand/latency under controlled workload conditions.
The lab fills a real gap. The follow-on phases (multi-producer, crypto inventory)
remain the right next steps.
