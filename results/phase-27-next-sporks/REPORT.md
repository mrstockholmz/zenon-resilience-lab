# Phase 27 — Next sporks on an isolated devnet (#277 EmbeddedRefundSpork, #280 ack recency)

Date: 2026-11-14. Lab session: kimi-head (lab lane). Devnet: isolated solo-pillar
chain 69 (`~/lab-devnet-p27`), RPC 127.0.0.1:48001 only, disposable devnet keys.

## What node operators need to know (short version)

1. **EmbeddedRefundSpork (fix for issue #271, merged as #277).** A failed
   contract-to-contract send (the live trigger: a bridge wrap of an Owned-pair
   token that is not burnable) stalls the Token contract's receive queue before
   the spork: the refund send carries no data, the embedded method lookup
   fails, the receive is never generated and every receive queued behind it
   waits. After the spork, the refund completes as a plain balance transfer,
   the failing receive finalizes as failed and the queue advances. Queues that
   stalled *before* activation recover once the spork is in force.
2. **Acknowledgement recency (fix for issue #227, merged as #280).** Once
   Dynamic Plasma is enforced, a user block must acknowledge a momentum at
   which every spork enforced at the frontier is already enforced. Enforced at
   pool admission, in the VM, and by the momentum verifier.
3. **Mixed networks.** A node that does not know an enforced spork does not
   silently fork: it validates everything up to the enforcement height and
   then terminates (exit status 2). Verified with the official v0.0.10 binary
   against a master devnet.

## Code under test

- go-zenon `dev` at `ab8ee8d` (merge of #280, includes #277 = `d610b48`).
  Worktree: `~/lab-go-zenon-master`.
- Lab patches (documented, devnet-only — consensus rule changes: none; they
  only select which on-chain spork entries the binary recognises and shrink
  time-challenge durations, mirroring the values the maintainers' own mock
  tests use):
  - `common/types/spork.go`: `BridgeAndLiquiditySpork` and
    `EmbeddedRefundSpork` ids = the hashes of the two sporks created on the
    devnet (same mechanism as a real release that hardcodes the governance
    broadcast hash).
  - `vm/constants/embedded.go`: `InitialBridgeAdministrator` = the genesis
    devnet spork key; `MinAdministratorDelay` 20, `MinSoftDelay` 10,
    `MinUnhaltDurationInMomentums` 5 (main-net values are epoch-scale and
    impossible on a devnet; the mock tests in `vm/embedded/tests/z_bridge_test.go`
    set the same 20/10/5).
- Correctness evidence on the **unmodified** tree: `go test -race -count=1`
  green on `./chain/... ./verifier/... ./pillar/...` and targeted
  `TestEmbeddedRefund_*` + `TestMomentumContent_StaleAck*` +
  `TestAccountPool_StaleAck*` in `./vm/embedded/tests/` (logs in the session
  notes).

## Method

- Built go-zenon master, booted a fresh solo-pillar devnet from the lab
  genesis (DP spork enforced at height 30).
- Created two sporks on-chain from the genesis spork address (CreateSpork /
  ActivateSpork), patched the ids into the lab build, restarted.
- Reproduced the exact #271 live trigger: bridge governance
  (orchestrator info → guardians (administrator delay) → TSS key (soft
  delay) → network + Owned pair registration), issued a mintable but
  **unburnable** token, wrapped it: the bridge's Burn to the Token contract
  fails with ErrPermissionDenied.
- Driver: `probe/` (Go, RPC-only, uses the disposable devnet mnemonic),
  scripts `run-fresh.sh`, `run-part3.sh`, verdicts `check-phase27.py`.
  Raw JSON: `raw/`.

## Results

(fill in from check-phase27.py output — appended below)

## Notes / honest limitations

- The bridge governance delays and unhalt window are lab-accelerated; the
  refund and recency rules themselves are unmodified.
- The pre-spork stall was observed on the **master** build (behaviour
  byte-identical to v0.0.10 per the #277 commit message); the v0.0.10 binary
  was used in the mixed-network check (c), where it cannot reach the stall
  because it stops at the enforcement height first.
