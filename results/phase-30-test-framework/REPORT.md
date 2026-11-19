# Phase 30a — Running 0x3639's test framework as written (linux/amd64 host)

Status: **in progress** (all-on, mainnet-like and fast done; P11 fast collected this session)

## 30b — ported lab cases on `test/spork-cases` (fork branch)

Branch `test/spork-cases` in `~/go-zenon-spork-cases` on top of his
`testing/framework` (`abe1fa6`), pushed to our fork. Suites, all on the
staged five-node devnet (see the in-branch `docs/test-results/` for
evidence):

| Suite | Commit | Result | Coverage |
|---|---|---|---|
| S-7/S-7b ack-recency (`sporkedge_test.go`) | `0682285` | PASS (staged 101 subcases + stock) | 191→195 |
| S-4/X-4 unknown binary + recovery (`sporkhalt_test.go`) | `4125350` | 4/4 PASS, 57 s | +1 (EmbeddedRefundSpork E7:S-4) |
| S-1/S-2/S-5 staged enforcement (`sporkstaged_test.go`) | `3f4edf5` | 6/6 subtests PASS (S-5 fixed + re-run standalone, see below) | 16 tokens, spork 2/8→5/8, total 198/501 |
| N-5 fault-peer starting point (`faultpeer_test.go` + `test/devnet/fault/`) | `c5bba33` | not yet run end to end (capability harness; overlay build + gofmt/vet verified) | — |

N-5 details: the phase-29 drip liar ported as a build overlay in his
`test/devnet/fast/` pattern (`overlay.sh` + `handler.go.in`, ZNN_LIAR-gated,
never compiled without the overlay). `TestFaultPeerDrip` (E2E_FAULT=1)
measures production rate with/without the liar and asserts production
continues (post-#306 expectation); it skips when the libp2p spork is live
(liar is legacy-devp2p only) and documents the remaining work (libp2p fault
peer, docker/devnet fault role). The phase-29 honest-negative result (this
liar's sessions end fast on both pre- and post-fix binaries) is written into
the case comment — it is a harness, not a fix discriminator.

S-1/S-2/S-5 details: run positioned before the first staged height
(`E2E_MIN_HEIGHT=12`); per spork (libp2p@20, DP@30, ptlc@60, ER@70, ERT@80)
the case asserts all five nodes share one momentum hash at H-1/H/H+1, no
container ever restarted, and — for DP — the format gate: v1 + zero resource
prices before, v2 + prices ≥ `dp.MinResourcePrice` after. S-5 restarts all
five nodes and checks the spork table survives and the pre-restart frontier
stays canonical. Raw logs: `raw/e2e-run-sporkstaged*.log`,
`raw/coverage-sporkstaged*.tsv`, node logs `raw/logs-staged-s125/`
(3 benign pillar eror lines: chain-start too-late and post-restart timestamp
races, his known kinds).

**Finding pinned (30b): the DP momentum-format switch lands one momentum
past the enforcement height** — the producer builds momentum N on its parent
N-1's spork state (`pillar/worker_momentum.go`), so momentum 30 on staged is
still v1 0/0 and the first v2 momentum is 31 (network accepts both; the
verifier applies the same parent-based rule). First S-5 attempt failed on
this wrong expectation in the case itself; fixed (check h+1) and S-5 re-ran
standalone PASS 14.6 s. Evidence: in-branch `docs/test-results/2026-11-19-spork-staged/`.

Not coverable on the stock staged genesis: the ER refund send semantics
(bridge admin delays are epoch-scale) — left to unit tests + phase 27 lab
evidence, documented in the case comment.

## What this is

0x3639 pushed a go-zenon test framework to his fork, branch `testing/framework`
(head `abe1fa69`, on top of dev `924e6b8`) on 2026-11-18. Not a PR yet. This phase
repeats his first run (docs/TESTING.md section 5) on a plain Debian 13 linux/amd64
host that already runs a mainnet znnd node, and records every place his instructions
did not work as written. His reference run: `docs/test-results/2026-11-18-924e6b8/`
(macOS, Colima, arm64). **Differences from his result files are the finding.**

## Environment

| | ours | his |
|---|---|---|
| Commit | `abe1fa69` (test/framework-run branch; +2 commits over his `924e6b8` base, both test-only) | `924e6b8` |
| Host | Debian 13, 8 cores, 16 GB, native docker (no VM) | macOS, Colima 4 CPU / 8 GiB |
| Binary | linux/amd64, Go 1.24.1, SHA-256 `01ee2e49…d3536` (build/linux/znnd) | linux/arm64, `0793b664…` |
| Network | five-node devnet on genesis variant `all-on` (P3 via up.sh logic + port overlay, see D1) | stock genesis, then variants |

## Procedure results (P-numbers per his docs/TESTING.md)

| Procedure | Result | Time | Notes |
|---|---|---|---|
| P1 `go build ./...` | PASS | ~2 min (cached) | |
| P1 `go vet ./...` | known | | exactly his six unkeyed-field warnings in p2p/legacy(+nat); nothing new at abe1fa6 |
| P1 `go test ./... -count=1` | **44/45 packages** | ~6 min | see F1 |
| P1 `devnet-keygen --verify-genesis` | PASS | | |
| P2 build-image.sh (GOIMG=golang:1.24.1-alpine) | PASS | ~4 min (warm volume caches) | commit hash in version string matches abe1fa69 |
| P3+P4 up.sh/smoke on `all-on` | **29/30 checks** | ~8 min | see D1, F2 |
| P9 normal driver on `all-on` | **100/100 cases PASS** | 797 s | e2e-run-all-on.log; same count as his arm64 all-on run |
| P9 destructive (TestHaltOnUnknownSpork SP-5..SP-7) | **3/3 PASS** | 139 s | every node stops one below enforcement height |
| P6 log collection | done | | 85 eror-lines on 5 nodes, all the two kinds he documents as unexplained (plasma contract gossip ordering) |
| P7 wipe | done | | devnet down -v, volumes removed |
| coverage after run | 191/501 | | same gap list as his (record.py merged 1 token) |
| P10 `mainnet-like` | **driver PASS** | 486 s | e2e-run-mainnet-like.log, exit=0; no DP groups live, spork-ladder refusals checked |
| P11 `fast` | **21/21 cases PASS** | driver 1182 s | fast image build ~2 min (warm caches); every normal group skipped by design, TestFast* only; coverage run-fast.tsv (79 tokens); node logs logs-fast/ (83 eror lines, dominated by the known benign "not enough plasma on account" contract-update kind) |

## Deviations from his instructions (D) and findings (F)

**D1 — P3 fails on any host that already runs a znnd node (port collision).**
His compose publishes `0.0.0.0:35997/35998` (rpc HTTP/WS) — the same ports a real
znnd node binds. On this box the mainnet node owns 35995/35997/35998, so `up.sh`
dies with `bind: address already in use` and the rpc container never starts.
Fix used: a local compose overlay `compose-ports.yml` (in this directory, **not**
committed) remapping rpc→36997/36998 and syncnode→36999/36001, applied with a
fourth `-f` after `compose.variant.yml`, replicating up.sh otherwise. Two sub-notes:
(a) plain list merging in compose *concatenates* ports, so the overlay needs
`ports: !override`; (b) `smoke.py` hard-codes ports 35997/35999 with no option, so
the run used `smoke-ported.py` (same checks, ports from SMOKE_RPC_PORT/SMOKE_SYNC_PORT
env). Suggestion for upstream: make the published ports env-configurable
(`"${DEVNET_RPC_PORT:-35997}:35997"`) and give smoke.py a `--port` flag.

**F1 — `go test ./...` is not green on a LAN with a live UPnP gateway.**
`p2p/legacy/nat TestUPNP_DDWRT` FAILs here: the test starts a fake IGD on multicast
SSDP (239.255.255.250:1900), but the LAN's real router (FritzBox 192.168.178.1)
also answers the M-SEARCH, and the code under test discovers the real device
(`upnp.service mismatch: got "IGDv2-IP2", want "IGDv1-IP1"`). P1 says "pass: every
command exits 0" — on a host on a consumer LAN this test is environment-dependent.
His Colima VM network presumably did not leak the multicast to the LAN.
Suggest: scope the SSDP probe to the test interface, or skip when a foreign IGD
answers. All other 44 packages pass.

**F2 — smoke cadence-10s fails at chain start, not at 29→30.**
29/30 checks pass on `all-on`; the one failure is `cadence-10s` with slow intervals
at heights 2→3 (20 s), 3→4 (30 s), 4→5 (20 s) — the first momentums after genesis,
while peers are still connecting (libp2p activates at 20). His stock-genesis run had
the analogous finding at heights 29→30 (one 20 s interval). Same class: early-chain
warm-up, not a stuck chain. One-chain, three-producers, all 8 sporks, DP version 2
from 31, sync state 2 on all five nodes: PASS.

## Parity statement

On a different OS/arch (linux/amd64 vs darwin/arm64), different container runtime
(native docker vs Colima) and through a port remap, his framework reproduced his own
results: 100/100 e2e cases on `all-on`, 3/3 destructive, 29/30 smoke (same single
check class failing). The framework is portable; the two rough edges for "anyone
with the repository, Go and docker" are the fixed ports (D1) and the UPnP test (F1).

## Raw evidence

- `raw/P1-static.log`, `raw/P1-gotest.log`, `raw/P1-env.txt`
- `raw/P2-build-image.log`
- `raw/P4-smoke-all-on.log` / `.json`, `smoke-ported.py`, `compose-ports.yml`
- `raw/e2e-run-all-on.log`, `raw/coverage-all-on.tsv`, `raw/e2e-destructive-all-on.log`
- `raw/logs-{pillar,pillar2,pillar3,rpc,observer}.log`
