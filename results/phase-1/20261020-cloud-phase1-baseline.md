# Cloud result — human:resilience-lab — mrstockholmz/zenon-resilience-lab@6db5306

session: https://claude.ai/code/session_015Z9d32xb7d9tCaU5zBYGmC
started: 2026-10-20T05:50:12Z
done-when: final report with exact commands + key output for every claim, ending with the CLOUD-RESULT line

> Evidence input, not a verdict (G1/G10): reproduce key claims locally before relying on them.

## Result

I ran **Phase 1 (gap check + baseline)** of the Zenon Resilience Lab on a local, isolated devnet with disposable identities only. I pinned go-zenon `dev` at commit `e89f91f68b922a65370b7ffd9cfe860045a99b29` (znnd `v0.0.9`, Go 1.24.7), brought up a single-pillar devnet, and **proved a full transaction round trip**: a self-send of 1 ZNN from a disposable genesis-funded account (BIP-44 index 3, `z1qp3yph55…`) was included at account height 2, and the matching receive was included at height 3, restoring the balance (verified via `getAccountInfoByAddress`). Both legs used the **PoW plasma fallback** (33.2 s send, 9.8 s receive on 4 cores), because the probe account had 0 fused plasma.

Two things it does **not** mean: (1) a single-producer devnet says nothing about decentralisation, fairness or consensus security — only resource mechanics; (2) the round trip ran at momentum heights 2–11, i.e. **before** the dynamic-plasma spork's `EnforcementHeight` of 30, so the PoW figures reflect the **static** plasma regime, not Dynamic Plasma.

I could **not** complete the external half of the gap check: the named adjacent repos (`sol-znn/go-zenon-fuzz`, `0x3639/testnet`, `TminusZ/zenon-developer-commons`) are not reachable from this session (GitHub API access is disabled here; web search did not surface them). That part is **UNVERIFIED**.

I also found a **local-only robustness issue** (not remote-reachable): the pinned `dev` committed devnet genesis (`docker/devnet/genesis.json`) **fails its own `CheckGenesis`**, so `znnd` refuses to boot from it unmodified (details under Data).

## Reproduce

```bash
# Environment: Linux 6.18, 4 CPU cores, 15 GiB RAM, Go 1.24.7 (go1.24.7 linux/amd64)
# 1. Pin go-zenon dev
git clone --branch dev --depth 1 https://github.com/zenon-network/go-zenon
git -C go-zenon rev-parse HEAD      # => e89f91f68b922a65370b7ffd9cfe860045a99b29
go build -C go-zenon -o build/znnd ./cmd/znnd    # znnd v0.0.9

# 2. Generate disposable devnet keys/config (fixed public dev mnemonic; password "devnet")
make -C go-zenon devnet-keys        # derives dev addrs; writes docker/devnet/*/config.json

# 3. Docker devnet is NOT usable here (daemon/socket unavailable) -> run znnd natively.
#    Data dir MUST be /root/.znn so the committed keyfile's embedded "Path" matches
#    the on-disk location (manager keys its map by the keyfile's embedded Path).
mkdir -p /root/.znn/wallet
cp go-zenon/docker/devnet/genesis.json          /root/.znn/genesis.json
cp -r go-zenon/docker/devnet/pillar/wallet/.    /root/.znn/wallet/
# config.json: single bootstrap pillar, MinPeers=0, RPC 127.0.0.1:35997, Producer index 0

# 4. Make the genesis valid (see Data for why) — LAB-LOCAL patch only:
#    add Plasma-contract QSR block = sum(Fusions)=3e12 and bump QSR totalSupply by 3e12.

# 5. Start + prove round trip
go-zenon/build/znnd --config /root/.znn/config.json --data /root/.znn
# momentum advances 1->2->3->...->12 (single producer, ~1 momentum / 30-80s)
cd roundtrip && go build -o rt . && ./rt            # SEND leg
./rt recv <send_block_hash>                         # RECEIVE leg
```

Pinned revisions: go-zenon `dev` = `e89f91f68b922a65370b7ffd9cfe860045a99b29` (2026-10-19; HEAD is a merge of PR #177 from `0x3639`). SDK: the go-zenon module itself (`wallet`, `chain/nom`, `pow`, `common/types`) at that same commit. go.mod declares Go 1.23; built with Go 1.24.7.

## Data

**Dev address table** (`make devnet-keys`, BIP-44 `m/44'/73404'/i'`, from committed public mnemonic):

| index | role | address |
|---|---|---|
| 0 | pillar 1 producer | z1qq9n7fpaqd8lpcljandzmx4xtku9w4ftwyg0mq |
| 3 | general dev account (probe/signer used) | z1qp3yph55qgresyytz83anynr2f4z39x2z3ej3e |
| 8,9 | general dev accounts | z1qpeet8dcjg0m6x6m3tg437wnc42aa2nez2fzth, z1qqcam4ycu0ta8333hx38r5j2z3ry9jjfxkc7t5 |

**Plasma activation (authoritative, from genesis spork config — not inferred):** genesis `ChainIdentifier: 69`. Spork `dynamic-plasma`, id `0000…0002`, `activated: true`, `enforcementHeight: 30`. Code path `chain/cache/spork.go:33`: a spork is active iff `Activated && EnforcementHeight <= momentum.Height`. `common/types/spork.go:47` maps `DynamicPlasmaSpork = "0000…0002"`. ⇒ **static plasma for momentum heights 1–29, Dynamic Plasma from height 30**. `libp2p` spork (id `…0001`) enforced from height 20.

**Round-trip measurements** (static-plasma regime, momentum < 30):

| leg | block type | acct height | PoW (ms) | included | notes |
|---|---|---|---|---|---|
| SEND 1 ZNN self | UserSend (2) | 2 | 33243 | yes | plasma=0 → PoW fallback; balance 50000→49999 ZNN |
| RECEIVE | UserReceive (3) | 3 | 9835 | yes | paired to send hash `f24210e4…`; balance restored to 50000 ZNN |

Post-round-trip `getAccountInfoByAddress`: height 3, ZNN raw `5000000000000` (restored). Receive block `fromBlockHash=f24210e4…`, `confirmationDetail` present. Frontier momentum height 12 at end.

**Local-only robustness finding — committed `dev` devnet genesis is invalid against its own validator.** Running each `Check*` on `docker/devnet/genesis.json` (via a throwaway `cmd/gcheck`):
```
CheckPlasmaInfo: invalid balance for z1qxemdeddedxplasma... Expected token zts1qsr... to be present
```
The genesis declares 3 `PlasmaConfig.Fusions` totalling `3e12` raw QSR but includes **no GenesisBlock** crediting the Plasma contract (`z1qxemdeddedxplasmaxxxxxxxxxxxxxxxxsctrp`) that QSR, so `CheckPlasmaInfo` fails and `znnd` aborts with `invalid genesis config. Failed to pass tests`. Adding that block then trips `CheckTokenTotalSupply` (declared QSR supply `1.160e15` ≠ sum of balances `1.163e15`) — i.e. the committed genesis is internally inconsistent by exactly the `3e12` of pre-fused plasma. Fix that makes it boot: add the Plasma-contract QSR block (`3e12`) **and** raise QSR `totalSupply` by `3e12`. Impact: anyone building the devnet from this `dev` commit via the documented path cannot start a node from the committed genesis without this patch. **Local only, not remote-reachable** (fails before any networking). UNVERIFIED whether `make devnet-up` is also broken (docker daemon unavailable here, so I could not test the docker path end-to-end).

## Gate

**PARTIAL.** Phase-1 gate = "if the suite already exists elsewhere, say so and switch the plan to contributing there."
- Baseline sub-goals **PASS**: revision pinned (full hash), plasma implementation + activation recorded authoritatively from genesis (static ≤29 / Dynamic ≥30), legacy env-keyfile handling replaced by a generated disposable identity, devnet built, transaction round trip proven with exact numbers.
- Gap-check sub-goal **UNVERIFIED/BLOCKED**: the external adjacent repos are unreachable from this session, so I cannot confirm or deny that a usable suite already exists elsewhere. The gate's switch/continue decision therefore cannot be made yet.

## Files

All created in session scratch (not committed; this worker is read-only and does not push). Contents of the two non-trivial ones:

1. `roundtrip/main.go` (~230 lines) — disposable-identity round-trip prover. Decrypts the committed producer keyfile with password `devnet` to recover the dev mnemonic, derives index 3, builds+signs a `UserSend` (PoW fallback via `embedded.plasma.getRequiredPoWForAccountBlock` + `pow.GetPoWNonce`), publishes via `ledger.publishRawTransaction`, polls `getFrontierAccountBlock` for inclusion; `recv <hash>` mode builds the matching `UserReceive`. Summary only (>200 lines); key constants: `rpcURL=http://127.0.0.1:35997`, `chainID=69`, `signIdx=3`.
2. `roundtrip/go.mod` — `module roundtrip; go 1.23; require go-zenon v0.0.0; replace => <pinned go-zenon checkout>`.
3. `/root/.znn/config.json` — single-pillar native config (localhost RPC 35997, `MinPeers:0`, producer index 0, no seeders/bootstrap).
4. `/root/.znn/genesis.json` — the committed devnet genesis **plus the lab-local validity patch** (Plasma-contract QSR block `3e12`; QSR `totalSupply` `1.160e15`→`1.163e15`). The upstream file in the go-zenon checkout was not modified.
5. `go-zenon/cmd/gcheck/main.go` — throwaway genesis validator (calls `genesis.Check*` individually) used to diagnose the boot failure.

## Next

Recommended next phase: **Phase 2 (smallest experiment)**, with two plan adjustments the owner should fold into STATE.md:
1. **Record the genesis finding and decide its disposition.** Either (a) open a minimal upstream fix adding the Plasma-contract balance + QSR supply to `docker/devnet/genesis.json`, or (b) carry a documented lab-local genesis. This is a concrete, reproducible local defect found during Phase 1 — a candidate "maintainer decision the results inform."
2. **Pin plasma regime explicitly in Phase 2.** Run two sub-matrices: momentum height <30 (static) and ≥30 (Dynamic Plasma), since the spork boundary materially changes plasma/PoW cost. Phase-1 numbers are static-regime only.
3. **Resolve the gap check first** (needs a session with GitHub read access, or the owner supplying the repos) before building any new suite under `suite/`, per the Phase-1 gate — avoid building a competing framework until existing suites are confirmed absent/insufficient.

Suggested STATE.md after owner review: `pinned_go_zenon: e89f91f68b922a65370b7ffd9cfe860045a99b29`, `pinned_sdk: <same commit>`, `plasma_mode: static<30 / dynamic>=30 (devnet genesis spork 0000…0002, enforcementHeight 30)`. Leave `next_phase` at 1 until the external gap check is completed, or bump to 2 if the owner accepts the gap check as blocked-by-access and the baseline as sufficient.

## PRIVATE-FINDING

None. No crash or flaw reachable by a remote party (peer, RPC client, or wallet) was observed. The one defect found (invalid committed devnet genesis) is local-only — it fails validation before any networking starts.

CLOUD-RESULT: partial
