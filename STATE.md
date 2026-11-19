# STATE — zenon-resilience-lab

## Current session state: 2026-11-19 ~20:35 CEST, phase 30b suite 3 done

- Done this session (lab lane, work item A + C): A — renewal spork 2be489e3
  still activated=false, eh=0 (direct RPC), fork-watch lockstep → no note.
  30b third suite: S-1/S-2/S-5 staged enforcement cases on branch
  `test/spork-cases` @ `3f4edf5` (worktree ~/go-zenon-spork-cases, pushed to
  our fork). TestSporkStagedEnforcement (test/e2e/sporkstaged_test.go):
  6/6 subtests PASS on the staged five-node devnet, run positioned before the
  first staged height (E2E_MIN_HEIGHT=12). Per spork (libp2p@20, DP@30,
  ptlc@60, ER@70, ERT@80): all five nodes share one momentum hash at
  H-1/H/H+1 (no fork), no container ever restarted; DP format gate pinned
  (v1 + zero prices before, v2 + prices ≥ dp.MinResourcePrice after); S-5
  restarts all five nodes — spork table survives, no resync.
- FINDING pinned: the DP momentum-format switch lands one momentum PAST the
  enforcement height — the producer builds momentum N on parent N-1's spork
  state (pillar/worker_momentum.go): momentum 30 on staged is v1 0/0, first
  v2 is 31. (First S-5 attempt failed on this wrong case expectation; fixed,
  re-ran standalone PASS 14.6 s. Documented in the case + in-branch results.)
- Coverage: +16 tokens (record.py), spork items 2/8→5/8, total 198/501.
  Evidence: in-branch docs/test-results/2026-11-19-spork-staged/ + copied
  here under results/phase-30-test-framework/.
- Run recipe: staged devnet needs the 5-RPC overlay
  (results/phase-30-test-framework/compose-5rpc.yml: 35991/36992/36993/36997/
  36994) + E2E_ALL_RPC + docker CLI (restart/inspect). Chain ticks 10 s per
  momentum — iteration is window-bound; wipe (down -v + up) to re-position.
- Nothing running. Devnet down -v (node logs raw/logs-staged-s125/, 3 benign
  pillar eror lines). Images kept: go-zenon-devnet:latest (+noref for S-4).
- NEXT SESSION: 30a P11 `fast` (FAST=1 build-image.sh, up.sh fast, overlay
  driver + fast e2e build per TESTING.md); then 30b N-5 drip-liar fault peer
  (#306, phase-29 probe as the starting point). ~/go-zenon still on
  test/framework-run (stray uncommitted coverage.tsv edit E1:PI-9 from P9).
