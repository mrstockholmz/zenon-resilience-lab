# Reset Procedure

How to reset the lab to a clean state and re-run from scratch.

## 1. Stop all lab nodes

```bash
# Stop any running znnd lab instances
pkill -f "znnd.*lab-devnet" 2>/dev/null || true
pkill -f "znnd.*lab-devnet-pillar" 2>/dev/null || true
```

## 2. Remove data directories

```bash
rm -rf ~/lab-devnet/
rm -rf ~/lab-devnet-pillar2/
rm -rf ~/lab-devnet-pillar3/
```

## 3. Rebuild from pinned go-zenon

```bash
cd ~/lab-go-zenon
git fetch origin
git checkout e89f91f68b922a65370b7ffd9cfe860045a99b29
make znnd
```

## 4. Recreate the lab genesis

The committed `docker/devnet/genesis.json` at `e89f91f` fails `CheckPlasmaInfo`
(3 Plasma fusions declared but no genesis block credits the Plasma contract).
The lab genesis adds 3e12 QSR to the plasma contract and raises QSR totalSupply.
See `results/phase-1/` for the full defect report.

Copy the lab-patched genesis into `~/lab-devnet/genesis.json`.

## 5. Recreate config

See `suite/README.md` for the full config template. Key points:
- RPC on 127.0.0.1 only
- No external peers
- `P2PBackend: "libp2p"` for multi-pillar setups
- Producer keyfile from the go-zenon devnet keyfile

## 6. Start the devnet

```bash
mkdir -p ~/lab-devnet
nice -n 10 ~/lab-go-zenon/build/znnd --data ~/lab-devnet &
```

Wait for sync (momentum height > 0):
```bash
curl -s http://127.0.0.1:57997 -X POST -H 'Content-Type: application/json' \
  -d '{"jsonrpc":"2.0","id":1,"method":"ledger.getFrontierMomentum","params":[]}'
```

## 7. Build labctl and run

```bash
cd ~/zenon-resilience-lab/suite
go build -o labctl ./cmd/labctl
./run-suite.sh all
```

## 8. Verify

```bash
python3 suite/check-regressions.py
# Expected: Summary: 20 PASS, 0 FAIL, 0 NOT TESTED
```
