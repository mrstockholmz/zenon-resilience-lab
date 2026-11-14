# Phase 27 — Next sporks on an isolated devnet (#277 EmbeddedRefundSpork, #280 ack recency)

Date: 2026-11-14. Lab session: kimi-head (lab lane). Devnets: isolated solo-pillar
chain 69 instances p27d (`~/lab-devnet-p27d`, RPC 127.0.0.1:48031) for (a)+(b)
and p27c (`~/lab-devnet-p27c`, RPC 127.0.0.1:48021) for (c2); disposable devnet
keys. Two earlier attempts (chain-1, p27b) failed on setup races; their evidence
is preserved under `raw/failed-p27b/` and `raw/part1.log` — see "Attempt log".

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
3. **Mixed networks.** An old node never follows a chain past an enforcement
   it cannot implement, in either of two ways: (i) if the chain enforces
   Dynamic Plasma, the old node cannot even decode the momentums (wire format
   change) and stalls at the genesis height; (ii) otherwise it validates
   everything up to the enforcement height and then terminates with exit
   status 2 ("Detected an unimplemented spork … Please upgrade your znnd
   binary"). Verified with the official v0.0.10 binary against a master
   devnet, both ways (c1, c2).

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
  scripts `run-p27d.sh` (a+b), `run-p27c-full.sh` (c2), `run-part3.sh` (c1),
  verdicts `check-phase27.py` (16 checks).

## Attempt log (honest history)

- **Chain-1 + repairs (00:33–02:18)** — first attempt; the pre-spork window
  was botched (pair confirmation raced the soft delay). Unit-test evidence
  (`b5`/`b6`) comes from this attempt, run on the unmodified tree at ab8ee8d.
- **p27b (10:57–11:42)** — full re-run; wrap #1 FATALed with "insufficient
  balance for transfer" because it raced the UNBURN mint receive (the mint
  credit is ledger-visible one beat after the issue; the probe's `tee` also
  swallowed the FATAL line), and the (b) fresh-ack controls raced account 4's
  funding receive. The (a) checks from this attempt are VOID; the b1/b3
  recency rejections in `failed-p27b/` are still valid (empty-account
  rule-isolated evidence).
- **p27d (19:27–20:25)** — corrected re-run: polls until the mint is
  ledger-visible before wrapping; receives the funding before the ack probes;
  captures all output with `2>&1 | tee`. All (a)+(b) evidence is from this run.
- **p27c (19:30–19:41)** — (c2) clean-exit run in a copied worktree
  (`~/lab-go-zenon-p27c`), genesis with libp2p@4000 / dynamic-plasma@5000 so
  the pre-enforcement wire form stays v0.0.10-decodable.
- **(c1)** — v0.0.10 vs the DP-enforced chain p27 (part3, 10:46).
  Raw JSON: `raw/`.

## Results

`python3 check-phase27.py` — **16/16 PASS, 0 FAIL, 0 NOT TESTED** (2026-11-14).

| check | verdict | evidence (raw/) |
|---|---|---|
| a1 pre-spork burn stuck at token-queue head | PASS | `refundstate-0-stalled.json` frontier 92: queue = [bridge Burn 43e7eab1…, contract send, 9985 UNBURN]; bridge holds only the 15 wrap fee; canary CANARY5 absent |
| a1 canary blocked behind the burn | PASS | `refundstate-0b-canary-blocked.json` frontier 96: canary Issue published but token not created |
| a1 no refund pre-spork | PASS | bridge balance 15 (fee only); burned 9985 stuck with the send |
| a2 stalled queue recovers after EmbeddedRefundSpork (enforcement height **103**, spork id e8d73c5e…) | PASS | `refundstate-1-recovered.json` frontier 107: queue empty, burn gone |
| a2 canary executes after recovery | PASS | `canaryTokenExists: true` at 107 |
| a2 refund deposited on bridge | PASS | bridge balance 10000 = 15 fee + 9985 refunded burn |
| a3 post-spork wrap #2: no stall, no burn stuck | PASS | `refundstate-2-wrap2.json` frontier 111: queue empty, bridge 20000, total supply constant 100000 |
| b1 stale ack h29 (pre-DP) rejected | PASS | `stale-ack-h29.json`; on the funded account the old older-than-previous rule fires first — rule-isolated evidence (empty account, recency message) in `failed-p27b/stale-ack-h29.json` |
| b2 fresh-ack control accepted | PASS | `fresh-ack-control.json` included |
| b3 block acking enforcement−1 rejected by the #280 recency rule | PASS | `stale-ack-enf-minus-1.json`: "momentum-acknowledged predates a spork enforced at the parent momentum: spork e8d73c5e… enforced at 103 but not at 95" (admission, before balance checks) |
| b4 fresh-ack after enforcement accepted | PASS | `fresh-ack-after-refund-spork.json` included |
| b5 unit tests chain/verifier/pillar, -race | PASS | `p27-unittests.log` (ab8ee8d, unmodified tree) |
| b6 unit tests vm/embedded ack-recency + refund, -race | PASS | `p27-vmtests.log` |
| c1 v0.0.10 on a DP-enforced chain: wire-level split | PASS | `mixed-v0010-result.json` + `part3.log`: connects, every momentum batch fails RLP decode (DP price fields), stuck at height 1 — old nodes partition off instantly, no silent fork |
| c2 v0.0.10 clean exit at unknown spork | PASS | `p27c-v0010-result.json` + `p27c-v0010-loglines.txt`: on chain p27c (libp2p@4000, DP@5000, unknown spork p27c-x enforced at 16) v0.0.10 validated momentums 1..15, refused momentum 16 — "Detected an unimplemented spork. Spork name `p27c-x` … Please upgrade your znnd binary" — exit status **2** |

### Failure modes observed (what operators will actually see)

- **Before EmbeddedRefundSpork enforcement** a failed contract-to-contract burn
  (bridge Burn of an unburnable token) leaves the Burn send unreceived at the
  head of the Token contract queue; every later send to the Token contract
  (here: a token Issue) waits behind it indefinitely. The bridge keeps only
  the wrap fee; the wrapped amount sits with the stuck send.
- **At/after enforcement** the Burn receive is moved to the first momentum
  whose spork state has the spork in force (height 103 here), finalizes as
  failed, and the refund lands as a plain balance transfer. Queues stalled
  *before* activation recover without any manual intervention.
- **Ack recency (#280)** is enforced at pool admission before balance checks
  (rejection message quotes the spork id and both heights), in the VM, and
  by the momentum verifier; unit tests on the unmodified tree cover the VM
  and verifier arms (`-race`).
- **Mixed networks**: an old node never follows a chain past an enforcement
  it cannot implement — it either cannot decode the momentums at all
  (Dynamic Plasma wire change, c1) or validates up to the enforcement
  height and terminates with exit status 2 (unknown spork, c2). Node
  operators: any enforced spork your binary does not know IS a hard
  partition signal, by design.

## Notes / honest limitations

- The bridge governance delays and unhalt window are lab-accelerated; the
  refund and recency rules themselves are unmodified.
- The pre-spork stall was observed on the **master** build (behaviour
  byte-identical to v0.0.10 per the #277 commit message); the v0.0.10 binary
  was used in the mixed-network check (c), where it cannot reach the stall
  because it stops at the enforcement height first.
