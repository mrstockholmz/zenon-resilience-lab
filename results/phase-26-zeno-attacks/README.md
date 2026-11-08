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
| spork-gating-active | read-only | settlement spork `…0004` active, domain live |
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

## What a devnet cannot show

- Real value at risk and adversarial pillar/executor behavior (single pillar,
  single executor, public seeds).
- Spork deactivation paths (would need governance control of the chain).
- Multi-executor domains and proposer rotation.
