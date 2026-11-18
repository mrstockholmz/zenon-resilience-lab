# STATE — zenon-resilience-lab

## Current session state: 2026-11-19 ~01:50 CEST, phase 30a (30b next)

- Done this session: work item A (renewal watch, no change, nothing recorded);
  phase 30a first half — 0x3639 test framework (branch testing/framework @ abe1fa69)
  run as written on linux/amd64. Results: results/phase-30-test-framework/
  (commit a394725, pushed): P1 build/vet/genesis-verify PASS (go test 44/45,
  TestUPNP_DDWRT fails — real FritzBox answers the test's multicast SSDP),
  P2 image abe1fa6 linux/amd64, P3+P4 all-on 29/30 smoke (cadence fails at h2-4,
  chain start), P9 all-on 100/100 in 797s, destructive SP-5..7 3/3 PASS, P6 logs
  (85 eror lines, his two known kinds), P7 wipe. FINDINGS: D1 his compose publishes
  35997/35998 which collide with any live znnd node (fixed with uncommitted local
  overlay results/phase-30-test-framework/compose-ports.yml + smoke-ported.py);
  F1 TestUPNP_DDWRT env-dependent; F2 cadence class same as his (different heights).
- Nothing running. Devnet torn down (down -v). Mainnet/testnet untouched.
- NEXT SESSION (work item B rest, then C): P10 mainnet-like driver run and P11 fast
  build+run (FAST=1 build-image.sh, up.sh fast, overlay driver). Reuse the port
  overlay + smoke-ported.py + E2E_RPC=http://localhost:36997,http://localhost:35991.
  Then work item C: branch test/spork-cases on top of testing/framework — S-4/X-4
  (phase 28: node without spork exits status 2 + recovery), S-1/2/5/7 for
  EmbeddedRefund/DynamicPlasma sporks on `staged` with E7 edge, then drip-liar N-5.
  Pin commit in this file. ~/go-zenon currently on branch test/framework-run.
