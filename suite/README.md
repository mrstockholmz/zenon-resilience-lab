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

## run-suite.sh

```bash
./run-suite.sh [phase2|phase3|phase4|check|all]
```

| Target | What it runs |
|---|---|
| `phase2` | Ordinary + burst + probe in current regime |
| `phase3` | Full 4-case matrix x 3 seeds (12 runs) |
| `phase4` | High-fused steady + burst x 3 seeds (comparison data) |
| `check` | Regression assertions (R1-R5) + CSV export |
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

Run: `python3 check-regressions.py ../results`

## Safety

- Devnet only (chain 69, isolated). Never touches mainnet/testnet/shared networks.
- Disposable dev keys from the committed go-zenon devnet keyfile.
- RPC on 127.0.0.1 only. No external traffic.
- Read `../SAFETY.md` before running anything.
