#!/bin/bash
# phase27 chain-C phase 2: patch p27c spork id, rebuild, activate bridge,
# then run the official v0.0.10 node against the master and watch it
# sync to the enforcement height and terminate.
# PRE: run-p27c-phase1.sh finished (raw/p27c-spork-bridge-id.txt exists).
set -u
cd ~/zenon-resilience-lab/results/phase-27-next-sporks
export LAB_RPC_URL=http://127.0.0.1:48021
P=probe/phase27probe
LOG=raw/p27c2.log
exec >> "$LOG" 2>&1
set -x

FM() { curl -s -m 5 -X POST http://127.0.0.1:48021 -H 'Content-Type: application/json' -d '{"jsonrpc":"2.0","id":1,"method":"ledger.getFrontierMomentum","params":[]}' | python3 -c "import json,sys; print(json.load(sys.stdin)['result']['height'])"; }

BRIDGE_ID=$(cat raw/p27c-spork-bridge-id.txt)
cd ~/lab-go-zenon-master
sed -i "s|BridgeAndLiquiditySpork = NewImplementedSpork(\"[0-9a-f]*\") // LAB p27 devnet spork|BridgeAndLiquiditySpork = NewImplementedSpork(\"$BRIDGE_ID\") // LAB p27 devnet spork|" common/types/spork.go
grep "LAB p27" common/types/spork.go
make znnd >> /tmp/p27c-build.log 2>&1
echo "p27c build exit=$?"

cd ~/lab-devnet-p27c && setsid nohup nice -n 10 ~/lab-go-zenon-master/build/znnd --data ~/lab-devnet-p27c >> ~/lab-devnet-p27c/boot.log 2>&1 < /dev/null &
sleep 15
cd ~/zenon-resilience-lab/results/phase-27-next-sporks
$P sporkactivate $BRIDGE_ID
ENF=$($P sporkwait p27-bridge)
echo "p27c bridge enforcement $ENF" | tee raw/p27c-bridge-enforcement.txt
$P waitfor $((ENF + 2))

# ---- v0.0.10 node ----
V=~/lab-devnet-p27c-v0010
ENODE=$(cd /tmp/enode && ./enode ~/lab-devnet-p27c/network-private-key 58021)
echo "p27c master enode: $ENODE"
rm -rf $V
mkdir -p $V
python3 - "$V" "$ENODE" << 'EOF'
import json, sys
v, enode = sys.argv[1], sys.argv[2]
import shutil
shutil.copy("/home/mrstockholmez/zenon-resilience-lab/suite/devnet/genesis.json", f"{v}/genesis.json")
# same spork shift as the master genesis
g = json.load(open(f"{v}/genesis.json"))
for s in g.get("SporkConfig", {}).get("Sporks", []):
    if s.get("Name") == "libp2p":
        s["EnforcementHeight"] = 4000
    if s.get("Name") == "dynamic-plasma":
        s["EnforcementHeight"] = 5000
json.dump(g, open(f"{v}/genesis.json", "w"), indent=4)
cfg = {
  "DataPath": v,
  "GenesisFile": f"{v}/genesis.json",
  "LogLevel": "info",
  "Name": "lab-p27c-v0010",
  "Net": {
    "ListenHost": "127.0.0.1",
    "ListenPort": 58022,
    "MinConnectedPeers": 1,
    "MinPeers": 1,
    "Seeders": [enode]
  },
  "RPC": {
    "EnableHTTP": True,
    "EnableWS": False,
    "Endpoints": [],
    "HTTPCors": [],
    "HTTPHost": "127.0.0.1",
    "HTTPPort": 48022,
    "HTTPVirtualHosts": ["localhost"],
    "WSHost": "127.0.0.1",
    "WSPort": 46022
  },
  "WalletPath": f"{v}/wallet"
}
json.dump(cfg, open(f"{v}/config.json", "w"), indent=4)
EOF
cd $V
setsid nohup nice -n 10 ~/lab-v0010/znnd-linux-amd64 --data $V > $V/boot.log 2>&1 < /dev/null &
VPID=$!
echo "p27c v0010 pid $VPID"

V_H=0
REACHED=0
for i in $(seq 1 100); do
  sleep 5
  if ! kill -0 $VPID 2>/dev/null; then
    echo "p27c v0010 exited after ~$((i*5))s"
    break
  fi
  V_H=$(curl -s -m 5 -X POST http://127.0.0.1:48022 -H 'Content-Type: application/json' -d '{"jsonrpc":"2.0","id":1,"method":"ledger.getFrontierMomentum","params":[]}' | python3 -c "import json,sys; print(json.load(sys.stdin)['result']['height'])" 2>/dev/null || echo 0)
  echo "t+$((i*5))s p27c master=$(FM) v0010=$V_H enf=$ENF"
  if [ "$V_H" -ge "$((ENF - 1))" ] 2>/dev/null; then
    REACHED=1
  fi
done
if kill -0 $VPID 2>/dev/null; then STILL=1; else STILL=0; fi
echo "p27c v0010 still_running=$STILL lastHeight=$V_H reached=$REACHED"
grep -i "spork\|unimplemented\|not implemented\|exit\|fatal" $V/boot.log $V/log/*.log 2>/dev/null | tail -12 | tee ~/zenon-resilience-lab/results/phase-27-next-sporks/raw/mixed-v0010b-loglines.txt

cd ~/zenon-resilience-lab/results/phase-27-next-sporks
python3 - "$V_H" "$ENF" "$REACHED" "$STILL" << 'EOF'
import json, sys
v_h, enf, reached, running = sys.argv[1:5]
result = {
    "chain": "p27c (master dev, libp2p @4000, dynamic-plasma @5000, p27-bridge @ENF)",
    "enforcementHeight": int(enf),
    "lastHeight": int(v_h),
    "preEnforcementSyncOk": reached == "1",
    "terminated": running == "0",
    "syncBlockedByWireFormat": False,
    "evidence": open("raw/mixed-v0010b-loglines.txt").read() if __import__("os").path.exists("raw/mixed-v0010b-loglines.txt") else "",
}
json.dump(result, open("raw/mixed-v0010b-result.json", "w"), indent=2)
EOF
echo "P27C DONE"
