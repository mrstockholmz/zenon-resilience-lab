# Suite — Reproducible Workload Generator

Go CLI (`labctl`) + shell runner (`run-suite.sh`) that generates measured transaction
workloads against an isolated Zenon devnet and records results as JSON.

## Prerequisites

| Dependency | Version | Notes |
|---|---|---|
| Go | >= 1.23 | builds labctl and go-zenon |
| go-zenon | `e89f91f68b922a65370b7ffd9cfe860045a99b29` | pinned; see STATE.md |
| Python 3 | >= 3.9 | for check-regressions.py, json2csv.py |
| znnd (lab build) | from pinned go-zenon | isolated data dir, RPC 127.0.0.1 only |

## Quick start (one command)

```bash
# 1. Clone and build go-zenon at the pinned revision
git clone https://github.com/zenon-network/go-zenon ~/lab-go-zenon
cd ~/lab-go-zenon
git checkout e89f91f68b922a65370b7ffd9cfe860045a99b29
make znnd        # or: go build -o build/znnd ./cmd/znnd

# 2. Create the lab devnet data dir
mkdir -p ~/lab-devnet
# Copy the lab genesis (with plasma-contract funding fix from phase 1).
# The committed docker/devnet/genesis.json at e89f91f FAILS CheckPlasmaInfo.
# The lab genesis adds 3e12 QSR to the plasma contract and raises QSR totalSupply.
# See results/phase-1/ for the full defect report.

# 3. Create config.json (RPC on 127.0.0.1 only, no external peers)
cat > ~/lab-devnet/config.json << 'EOF'
{
  "DataPath": "/home/user/lab-devnet",
  "GenesisFile": "/home/user/lab-devnet/genesis.json",
  "LogLevel": "info",
  "Name": "lab-devnet-pillar",
  "Net": {
    "BootstrapPeers": [],
    "ListenHost": "127.0.0.1",
    "ListenPort": 56995,
    "MinConnectedPeers": 0,
    "MinPeers": 0,
    "Seeders": []
  },
  "Producer": {
    "Address": "z1qq9n7fpaqd8lpcljandzmx4xtku9w4ftwyg0mq",
    "Index": 0,
    "KeyFilePath": "z1qq9n7fpaqd8lpcljandzmx4xtku9w4ftwyg0mq",
    "Password": "devnet"
  },
  "RPC": {
    "EnableHTTP": true,
    "EnableWS": false,
    "Endpoints": [],
    "HTTPCors": [],
    "HTTPHost": "127.0.0.1",
    "HTTPPort": 57997,
    "HTTPVirtualHosts": ["localhost"],
    "WSHost": "127.0.0.1",
    "WSPort": 57998
  }
}
EOF

# 4. Start the devnet (under nice, local only)
nice -n 10 ~/lab-go-zenon/build/znnd --data ~/lab-devnet &

# 5. Build labctl
cd suite/
go build -o labctl ./cmd/labctl

# 6. Verify
./labctl info
# Should show: momentum_height, plasma_regime, agent address, plasma balance

# 7. Run the suite
./run-suite.sh all
```

## labctl commands

| Command | Description |
|---|---|
| `info` | Node state: height, regime, agent plasma |
| `ordinary <n> <amount>` | n sequential self-transfers (ordinary workload) |
| `burst <n> <amount>` | n parallel self-transfers without waiting (burst workload) |
| `probe <seconds>` | Lightly-provisioned probe: periodic small sends |
| `fuse <qsr>` | Fuse QSR for plasma on the agent account |
| `fund <index> <znn>` | Send ZNN from agent to derived account[index] |
| `fusefor <index> <qsr>` | Fuse QSR with beneficiary=derived account[index] |
| `receive <index>` | Receive pending blocks on derived account[index] |
| `waitfor <height>` | Block until momentum height >= target |
| `matrix <case> <seed>` | Run one phase-3 matrix case (low/high x steady/burst x 1/multi) |
| `hvburst <n> <amt>` | High-volume burst with per-momentum price tracking (phase 6) |
| `sustain <secs> <accounts>` | Sustained multi-account burst for repricing tests (phase 6) |
| `contend <n> <txPerAcct> <amt>` | Multi-sender contention with plasma gradient (phase 7) |
| `deplete <idx> <qsr> <maxTx>` | Plasma depletion + regeneration test (phase 8) |
| `powlat <n> <amt>` | PoW latency measurement from N unfused accounts (phase 9) |
| `powburst <n> <secs> <amt>` | PoW repricing attempt via sustained burst (phase 9) |
| `mixcontend <fused> <unfused> <txPerAcct> <amt>` | Mixed fused/unfused contention (phase 11) |
| `saturate <n> <txPerAcct> <qsr> <amt>` | Saturation test: N accounts with equal plasma exceed 200 tx/momentum limit (phase 12) |
| `mixsaturate <fused> <unfused> <fTx> <uTx> <amt>` | Mixed fused/unfused saturation (phase 13) |
| `overpay <groups> <accts> <txs> <amt>` | Content selector price priority test (phase 14) |
| `regen <idx> <qsr>` | Plasma regeneration rate test (phase 15) |
| `txtype <idx> <qsr>` | Transaction type diversity test (phase 16) |
| `poolcap <idx> <qsr> <maxTx>` | Mempool cap test (phase 17) |
| `frontconf <idx> <qsr>` | Frontier conflict resolution test (phase 17) |
| `datasize <idx> <qsr>` | Data payload size limits + plasma cost formula (phase 18) |

## run-suite.sh

```bash
./run-suite.sh [phase2|phase3|phase4|phase6|phase7|phase8|phase9|phase11|phase12|phase13|phase15|phase16|phase17|phase18|check|all]
```

| Target | What it runs |
|---|---|
| `phase2` | Ordinary + burst + probe in current regime |
| `phase3` | Full 4-case matrix x 3 seeds (12 runs) |
| `phase4` | High-fused steady + burst x 3 seeds (comparison data) |
| `phase6` | hvburst 200 txs x 3 seeds (repricing test) |
| `phase7` | contend 10 accounts x 30 txs x 3 seeds (contention test) |
| `phase8` | deplete 100 QSR x 3 seeds (plasma depletion/regeneration) |
| `phase9` | powlat 14 accounts x 3 seeds (PoW latency measurement) |
| `phase11` | mixcontend 5 fused + 5 unfused x 3 reps (mixed contention) |
| `phase12` | saturate configs A/B/C (200-tx limit saturation behavior) |
| `phase13` | mixsaturate 20 fused + 10 unfused x 3 seeds (mixed saturation) |
| `phase15` | regen 100/50/10 QSR x 4 seeds (plasma regeneration rate) |
| `phase16` | txtype 200 QSR x 3 seeds (transaction type plasma costs) |
| `phase17` | poolcap + frontconf x 3 seeds (mempool cap + frontier conflict) |
| `phase18` | datasize 3000 QSR x 3 seeds (data payload size limits) |
| `check` | Regression assertions (R1-R15) + CSV export |
| `all` | phase2 + phase3 + phase4 + check |

Results are written to `../results/phase-N/*.json`.
CSV summary: `../results/summary.csv`.

## Regression assertions

`check-regressions.py` verifies five invariants against the raw JSON results:

| ID | Assertion |
|---|---|
| R1 | Fused plasma + steady sequential = 100% inclusion |
| R2 | Fused plasma + burst = 100% inclusion (when plasma available) |
| R3 | Unfused burst has < 100% inclusion (account-chain conflict is real) |
| R4 | Fused plasma eliminates PoW for simple sends |
| R5 | Each simple self-transfer consumes exactly 21000 plasma |
| R6 | Under contention, plasma availability correlates with inclusion rate (Spearman rho > 0) |
| R7 | Plasma depletes by exactly 21000 per simple self-transfer and regenerates to full fused amount after confirmation |
| R8 | PoW transactions from unfused accounts have 100% inclusion when sent sequentially (1 tx per account) |
| R9 | Mixed fused/unfused contention: fused 100% inclusion with 0 PoW, unfused 100% inclusion under capacity |
| R10 | Saturation: no momentum exceeds 200 txs; excess is queued not rejected (when plasma sufficient) |
| R11 | Mixed saturation: no plasma-based prioritization at floor prices; 100% inclusion |
| R12 | Plasma regenerates to full fused amount within 1 momentum after depletion |
| R13 | Transaction type plasma costs match go-zenon source constants (self=21000, data=21000+68/B, contract=52500) |
| R14 | Mempool cap is 500 uncommitted blocks per account; frontier conflict follows price-then-hash rule |
| R15 | Data payload plasma cost = 21000+68*N for N ≤ 16384; >16KB rejected with "account-block data field is too big" |

Run: `python3 check-regressions.py ../results`

All R1-R15 assertions passing as of 2026-10-24 (phase 18).

## Safety

- Devnet only (chain 69, isolated). Never touches mainnet/testnet/shared networks.
- Disposable dev keys from the committed go-zenon devnet keyfile.
- RPC on 127.0.0.1 only. No external traffic.
- Read `../SAFETY.md` before running anything.
