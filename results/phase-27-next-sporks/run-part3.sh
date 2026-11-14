#!/bin/bash
# phase27 part 3: mixed devnet — official go-zenon v0.0.10 binary peers with the
# master devnet, which has already enforced a spork v0.0.10 does not know.
# Expected: v0.0.10 validates master-produced momentums up to the enforcement
# height, then terminates (exit 2, unknown enforced spork).
set -u
cd ~/zenon-resilience-lab/results/phase-27-next-sporks
P=probe/phase27probe
LOG=raw/part3.log
exec >> "$LOG" 2>&1
set -x

V=~/lab-devnet-p27-v0010
MASTER_PEER=$(cd /tmp/enode && ./enode ~/lab-devnet-p27/network-private-key 58001)
echo "master enode: $MASTER_PEER"

# enforcement height of the FIRST spork v0.0.10 does not know (p27-bridge).
# v0.0.10 must validate up to here and then terminate. (p27-refund enforces
# later at 969 on this chain; the bridge spork is the binding one.)
ENF=$(cat raw/bridge-enforcement.txt)

# 1. isolate the master: drop the stale bootstrap peers inherited from the
#    long-lived lab config, keep everything else
python3 - << 'EOF'
import json
p = "/home/mrstockholmez/lab-devnet-p27/config.json"
cfg = json.load(open(p))
cfg["Net"]["BootstrapPeers"] = []
# v0.0.10 is legacy devp2p only (no libp2p at all): pin the master to the
# legacy backend so the two can peer; consensus is unaffected by the backend.
cfg["Net"]["P2PBackend"] = "legacy"
cfg["Net"]["MinConnectedPeers"] = 0
cfg["Net"]["MinPeers"] = 0
json.dump(cfg, open(p, "w"), indent=4)
print("master config isolated")
EOF
pkill -f "build/znnd --data /home/mrstockholmez/lab-devnet-p27" || true
sleep 6
cd ~/lab-devnet-p27 && setsid nohup nice -n 10 ~/lab-go-zenon-master/build/znnd --data ~/lab-devnet-p27 >> ~/lab-devnet-p27/boot.log 2>&1 < /dev/null &
sleep 20
curl -s -m 5 -X POST http://127.0.0.1:48001 -H 'Content-Type: application/json' -d '{"jsonrpc":"2.0","id":1,"method":"ledger.getFrontierMomentum","params":[]}' | python3 -c "import json,sys; print('master back at', json.load(sys.stdin)['result']['height'])"

# 2. v0.0.10 node setup
rm -rf $V
mkdir -p $V
cp ~/zenon-resilience-lab/suite/devnet/genesis.json $V/genesis.json
python3 - "$V" "$MASTER_PEER" << 'EOF'
import json, sys
v, peer = sys.argv[1], sys.argv[2]
cfg = {
  "DataPath": v,
  "GenesisFile": f"{v}/genesis.json",
  "LogLevel": "info",
  "Name": "lab-p27-v0010",
  "Net": {
    "ListenHost": "127.0.0.1",
    "ListenPort": 58002,
    "MinConnectedPeers": 1,
    "MinPeers": 1,
    "Seeders": [peer],
  },
  "RPC": {
    "EnableHTTP": True,
    "EnableWS": False,
    "Endpoints": [],
    "HTTPCors": [],
    "HTTPHost": "127.0.0.1",
    "HTTPPort": 48002,
    "HTTPVirtualHosts": ["localhost"],
    "WSHost": "127.0.0.1",
    "WSPort": 46002
  },
  "WalletPath": f"{v}/wallet"
}
json.dump(cfg, open(f"{v}/config.json", "w"), indent=4)
EOF

# 3. start the official v0.0.10 binary and watch it
cd $V
setsid nohup nice -n 10 ~/lab-v0010/znnd-linux-amd64 --data $V > $V/boot.log 2>&1 < /dev/null &
VPID=$!
echo "v0010 pid $VPID"

MASTER_H=0
V_H=0
REACHED=0
for i in $(seq 1 120); do
  sleep 5
  if ! kill -0 $VPID 2>/dev/null; then
    echo "v0010 process exited after ~$((i*5))s"
    break
  fi
  MASTER_H=$(curl -s -m 5 -X POST http://127.0.0.1:48001 -H 'Content-Type: application/json' -d '{"jsonrpc":"2.0","id":1,"method":"ledger.getFrontierMomentum","params":[]}' | python3 -c "import json,sys; print(json.load(sys.stdin)['result']['height'])" 2>/dev/null || echo 0)
  V_H=$(curl -s -m 5 -X POST http://127.0.0.1:48002 -H 'Content-Type: application/json' -d '{"jsonrpc":"2.0","id":1,"method":"ledger.getFrontierMomentum","params":[]}' | python3 -c "import json,sys; print(json.load(sys.stdin)['result']['height'])" 2>/dev/null || echo 0)
  echo "t+$((i*5))s master=$MASTER_H v0010=$V_H enf=$ENF"
  if [ "$V_H" -ge "$((ENF - 1))" ] 2>/dev/null; then
    REACHED=1
    # it should die at/shortly after ENF; keep polling to confirm termination
  fi
done

EXIT_CODE=$(wait $VPID 2>/dev/null; echo $?)
# setsid detaches; wait won't work — capture via /proc instead
if kill -0 $VPID 2>/dev/null; then
  STILL_RUNNING=1
else
  STILL_RUNNING=0
fi
echo "v0010 still_running=$STILL_RUNNING exit_code=$EXIT_CODE lastHeight=$V_H reachedPreEnforcement=$REACHED"

grep -i "spork\|terminat\|unimplemented\|fatal\|panic" $V/boot.log $V/log/error/zenon.error.log 2>/dev/null | tail -20 | tee raw/mixed-v0010-loglines.txt

python3 - "$V_H" "$ENF" "$REACHED" "$STILL_RUNNING" << 'EOF'
import json, sys
v_h, enf, reached, running = sys.argv[1:5]
result = {
    "enforcementHeight": int(enf),
    "lastHeight": int(v_h),
    "preEnforcementSyncOk": reached == "1",
    "terminated": running == "0",
    "evidence": open("raw/mixed-v0010-loglines.txt").read() if __import__("os").path.exists("raw/mixed-v0010-loglines.txt") else "",
}
json.dump(result, open("raw/mixed-v0010-result.json", "w"), indent=2)
EOF

echo "PART3 DONE"
