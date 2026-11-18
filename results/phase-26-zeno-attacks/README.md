# Phase 26 — Zeno settlement attack checks (mission M5)

Executable negative tests ("attacks") against the Zeno settlement contract on
the live local devnet (chain 69, domain 1). Each check prints
`PASS / FAIL / NOT TESTED`. PASS means the chain rejected the attack or the
invariant holds on-chain; FAIL means the attack went through (that would be a
reportable break — private channel first); NOT TESTED means this substrate
cannot exercise the case and the settlement branch's own unit tests are the
authority.

The checks implement the invariant map from the M1 review of
`digitalSloth/go-zenon` `feature/settlement` @ 938fca3
(`notes/projects/zeno-integration.md` in the operator workspace):

| check | attack | expected |
|---|---|---|
| spork-gating-active | read-only | settlement spork `…0005` active, domain live |
| deposit-while-spork-off | deposit with the spork deactivated | NOT TESTED live (needs a pillar-signed spork tx; would halt the shared devnet's executor). Branch unit test `RejectedBeforeSpork` is the authority |
| fee-accounting-totals | read-only | `burned + credited <= collected`, `rewards <= credited` |
| custody-conservation | read-only | `totalReleased + pendingReserve <= totalDeposited` |
| deposit-plasma-limit | Deposit with `plasmaLimit = MaxInputGas+1` | pool rejection (`ErrForbiddenParam`) |
| calll2-payload-limit | CallL2 with payload `MaxCallPayload+1` | pool rejection |
| batch-replay-executor | executor resubmits the exact commitment of an old batch | receive rejection (batchId chaining) |
| batch-replay-nonexecutor | same commitment from a non-executor | pool rejection (`ErrPermissionDenied`) |
| withdrawal-early-relay | RelayMessage before the batch finalizes | receive rejection (`ErrForbiddenParam`) |
| double-withdrawal | second RelayMessage of the same outboxId | receive rejection (`ErrSettlementOutboxProcessed`) |
| release-rate-limit | two ReleaseWithdrawals in one momentum | second rejected (`ErrUpdateTooRecent`); if the second lands in a later momentum the live verdict is NOT TESTED (timing) — unit tests cover the guard |

## Prerequisites

- The Zeno devnet on nummer13, LOCAL layout: `~/zeno-local/zeno/zeno-devnet`,
  brought up with `ZENO_DEVNET_LOCAL=1 scripts/devnet.sh up` (see
  `notes/projects/log/zeno-integration.log.md` for the port remap 359xx→55xxx
  and the compose-2.26 executor-key bind-mount workaround).
- Executor and gateway containers running (`docker ps` shows
  `zeno-devnet-executor-1` / `zeno-devnet-gateway-1`).
- The operator workspace's node deps inside the zeno-devnet checkout
  (`npm ci` has been run there once).

## Run

```sh
results/phase-26-zeno-attacks/run-zeno-attacks.sh
```

The runner stages `zeno-attacks.ts` into the zeno-devnet checkout's `flows/`
directory (the canonical copy is the one in this repo) and runs it with tsx.
Runtime is roughly 15–25 minutes: it deploys a vault contract, deposits,
triggers a real withdrawal, then races the delay/replay/rate-limit guards, and
waits one `WithdrawalDelay` (30 momenta) for batch finalization.

Exit code 0 = no FAIL. Each run appends a timestamped log to `results/`.

## Runs

- **2026-11-08 03:08 CEST, settlement 938fca3** (first run, fresh chain):
  9 PASS / 0 FAIL / 2 NOT TESTED, 16m22s, exit 0. Fees at that point:
  burned 7,909,200 + credited 18,454,800 = collected 26,364,000;
  deposited=100,000,000 released=40,000,000. Log:
  `results/run-20261108-025228.log`.
- **2026-11-12 12:53–13:12 CEST, settlement 2835c436** (MaxInputGas lowered
  1e9 → 1e8; devnet redeployed on the surviving chain at height ~55.4k):
  9 PASS / 0 FAIL / 2 NOT TESTED, ~19m, exit 0. `MAX_INPUT_GAS` constant in
  `zeno-attacks.ts` updated to 100,000,000 for this run; the
  deposit-plasma-limit check now probes 100,000,001 and is still pool-rejected
  (`forbidden parameter`). Fees at that point: burned 11,982,600 +
  credited 27,959,400 = collected 39,942,000 (chain survived the binary swap,
  so counters are cumulative); deposited=200,000,000 released=80,000,000.
  Log: `results/run-20261112-125259.log`.
- **2026-11-18 03:13–03:29 CET, settlement a8183083** (branch rebased; the
  offchain-execution settlement spork moved from placeholder `…0004` to
  `…0005` because `…0004` is EmbeddedRefundSpork upstream, plus a spork-ladder
  test covering all 64 combinations; fresh chain at the new head, executor and
  console pins unchanged): **9 PASS / 0 FAIL / 2 NOT TESTED, 16m26s, exit 0**.
  Suite updated: `SETTLEMENT_SPORK_ID` now `…0005`, and the two batch-replay
  checks moved after the withdrawal flow (on a fresh chain no batches exist
  until the first withdrawal is submitted — the 02:54 run failed both replay
  checks on `no batches on the domain`, an ordering artifact, not a contract
  break; both PASS after the move). The re-deploy of the vault contract in a
  repeat run works because finalize consumes the deploy record and re-start
  is accepted. Batch 11 replay rejected (receive failed, count unchanged at
  12); non-executor replay pool-rejected (`address cannot call this method`).
  Fees: burned 0 + credited 0 = collected 13,578,000 (rewards not yet
  distributed); deposited=100,000,000 released=40,000,000 per the fresh-chain
  run's own deposit; conservation held at the end. Logs:
  `results/run-20261118-025422.log` (first, ordering artifact),
  `results/run-20261118-031304.log` (green).

## What a devnet cannot show

- Real value at risk and adversarial pillar/executor behavior (single pillar,
  single executor, public seeds).
- Spork deactivation paths (would need governance control of the chain).
- Multi-executor domains and proposer rotation.
