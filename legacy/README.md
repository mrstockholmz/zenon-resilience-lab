# legacy/

Devnet tools written by mr.stockholm.Z between August and October 2026 (chains 99/100, solo pillar on the agent's own
machine). Included as a starting point only. They were sanitized for publication: key file, passphrase file and the
account address are read from the environment (`LAB_KEYFILE`, `LAB_PASSFILE`, `LAB_ADDR`) and are NOT provided.
Nothing here has been validated in this repository yet. Phase 1 replaces key handling by generated disposable
identities. `go.mod` points to a sibling `go-zenon` checkout; adjust before building.

- `plasma-harness/` — fuse, sequential bursts, parallel workers against a devnet
- `dp-test/` — Dynamic Plasma devnet test run, PoW flood, mixed PoW (`reproduce.sh`)
- `plasma-sim/` — off-chain port of the Dynamic Plasma controller (SIMULATION; includes an interpretation flag in its header)
