# Plan

**Question.** Under a pinned Zenon client configuration, how do Plasma allocation and automated
transaction demand affect confirmation latency, resource cost, and the ability of a lightly
provisioned participant to transact?

**Hypothesis (a judgement, not a claim).** Comparing resource access under different demand patterns
will uncover actionable limits or a reusable regression baseline. If the useful suite already exists
elsewhere (sol-znn/go-zenon-fuzz, 0x3639/testnet, existing simulations), contribute there instead of
building a competing framework.

## Phases (one cloud session of about 90 minutes each, run in order)

### 1 — Gap check and baseline
Survey adjacent work (sol-znn/go-zenon-fuzz, 0x3639/testnet, TminusZ/zenon-developer-commons plasma papers,
`legacy/`). Pin go-zenon revision, SDK revision, genesis, config. Record exactly which Plasma implementation
and activation settings apply (static plasma vs Dynamic Plasma spork); do NOT infer activation from testnet
config fields. Replace `legacy/` key handling by generated disposable identities. Build a devnet from the pin and
prove a transaction round trip. Gate: if the suite already exists elsewhere, say so and switch the plan to
contributing there.

### 2 — Smallest experiment
Isolated single-producer devnet. One ordinary workload and one burst workload, fixed seeds. Count attempted,
submitted, included, rejected, unresolved SEPARATELY. Record the generator's own CPU and request rate. If the VM
cannot run the topology, report the narrower scope first. Gate: no reproducible baseline after phases 1+2 → narrow
to one resource scenario or document the blocker.

### 3 — Resource matrix
Low vs higher fused resources x steady vs burst demand x one vs several sending accounts. A lightly provisioned
probe account runs throughout. Keep TOTAL resources comparable when testing account splitting; test increased total
resources separately. Record PoW fallback behaviour where supported.

### 4 — Comparison across configurations
Repeat the phase-3 matrix on baseline and Dynamic Plasma revisions only if both run with documented semantics.
Otherwise deliver the baseline plus an explicit dependency list. Never present simulated output as chain results.

### 5 — Package for reproduction
One command to run the suite, raw results, exact config, concise report, regression assertions where a real
invariant is established. Repeat a representative case at least 3 times and report variation.

### 6 — Pick the next experiment
Name one maintainer or builder decision the results inform. Prioritise next work by observed consequences. Draft
a feedback request (the agent posts only through post.py).

## Measurements
Inclusion latency distribution (p50/p90/p99 + censoring), sustained completed work, rejection reasons, unresolved
outcomes, CPU, peak memory, PoW effort where observable. Send inclusion and receive completion are separate events.
Fixed workload seeds. A percentile over only successful transactions must carry the failure/unresolved rate.
A single-producer devnet tests resource mechanics only; it says nothing about decentralisation, fairness or
consensus security.

## Done / continue / stop
Done when another developer can reproduce at least one meaningful result from the pinned inputs, the report states
its limits, and the suite answers a concrete regression or configuration question.
Continue when an outside developer reproduces or uses a result, asks for an extension, or a result shows a material
issue with a clear engineering response. Track independent reproductions, maintainer decisions informed, accepted
regression cases, cost per experiment. Comments and generated reports are not impact.
Stop after 6 sessions without a useful result, or when the cloud budget is gone; then compare alternatives again.

## Follow-on (only after phase 6)
Bounded investigation of AI and cryptographic resilience: dependency inventory of signature/hash use in the pinned
code, migration design questions, small compatibility benchmark only if the inventory yields a real question.
No speculative prose, no promise of a quantum-safe release.
