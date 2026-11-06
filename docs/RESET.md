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
cd ../lab-go-zenon   # sibling of this repo (see suite/README.md)
git fetch origin
git checkout e89f91f68b922a65370b7ffd9cfe860045a99b29
make znnd
```

## 4. Recreate the lab devnet data dir

One command (from the repo root):

```bash
scripts/setup-devnet.sh --go-zenon ../lab-go-zenon
```

This copies the committed lab genesis (`suite/devnet/genesis.json`) — the
stock `docker/devnet/genesis.json` at `e89f91f` fails `CheckPlasmaInfo`
(3 Plasma fusions declared but no genesis block credits the Plasma contract;
the lab genesis adds 3e12 QSR to the plasma contract and raises QSR
totalSupply. See `results/phase-1/`) — plus the producer keyfile from the
go-zenon checkout, patches the keyfile's embedded `Path` field to the new
location, and writes `config.json`.

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
