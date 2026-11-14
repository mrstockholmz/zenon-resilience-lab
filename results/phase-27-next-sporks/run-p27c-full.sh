#!/bin/bash
# phase27 chain-C full run (c2): verify the CLEAN old-node behavior against
# go-zenon master on a fresh isolated devnet:
#   - master chain with libp2p @4000, dynamic-plasma @5000 (genesis-shifted),
#     plus one spork ("p27c-x") created and activated early (~15)
#   - official v0.0.10 binary syncs the chain until the momentum where the
#     unknown spork is enforced, then terminates (exit status 2) instead of
#     forking (chain.GotAllActiveSporksImplemented)
# Uses the copied worktree ~/lab-go-zenon-p27c exclusively (the master tree
# is busy with p27d). Ports: master RPC 48021/net 58021, v0010 RPC 48022/net 58022.
set -u
cd ~/zenon-resilience-lab/results/phase-27-next-sporks
export LAB_RPC_URL=http://127.0.0.1:48021
P=probe/phase27probe
LOG=raw/p27c-full.log
exec >> "$LOG" 2>&1
set -x

M=~/lab-devnet-p27c
V=~/lab-devnet-p27c-v0010
W=~/lab-go-zenon-p27c

FM() { curl -s -m 5 -X POST http://127.0.0.1:48021 -H 'Content-Type: application/json' -d '{"jsonrpc":"2.0","id":1,"method":"ledger.getFrontierMomentum","params":[]}' | python3 -c "import json,sys; print(json.load(sys.stdin)['result']['height'])" 2>/dev/null || echo 0; }
VH() { curl -s -m 5 -X POST http://127.0.0.1:48022 -H 'Content-Type: application/json' -d '{"jsonrpc":"2.0","id":1,"method":"ledger.getFrontierMomentum","params":[]}' | python3 -c "import json,sys; print(json.load(sys.stdin)['result']['height'])" 2>/dev/null || echo 0; }
stop_master() { PID=$(ps aux | grep 'lab-devnet-p27[c]' | grep -v v0010 | awk '{print $2}'); [ -n "$PID" ] && kill $PID 2>/dev/null; sleep 6; }

# ---------- C1: genesis+config, boot, create spork ----------
stop_master
rm -rf $M
mkdir -p $M/wallet
python3 - "$M" << 'EOF'
import json, sys, shutil
d = sys.argv[1]
g = json.load(open("/home/mrstockholmez/zenon-resilience-lab/suite/devnet/genesis.json"))
for s in g.get("SporkConfig", {}).get("Sporks", []):
    if s.get("Name") == "libp2p":
        s["EnforcementHeight"] = 4000
    if s.get("Name") == "dynamic-plasma":
        s["EnforcementHeight"] = 5000
json.dump(g, open(f"{d}/genesis.json", "w"), indent=4)
shutil.copy("/home/mrstockholmez/go-zenon/docker/devnet/pillar/wallet/z1qq9n7fpaqd8lpcljandzmx4xtku9w4ftwyg0mq", f"{d}/wallet/")
p = f"{d}/wallet/z1qq9n7fpaqd8lpcljandzmx4xtku9w4ftwyg0mq"
k = json.load(open(p)); k["Path"] = p; json.dump(k, open(p, "w"), indent=4)
cfg = json.load(open("/home/mrstockholmez/lab-devnet/config.json"))
cfg["DataPath"] = d
cfg["GenesisFile"] = f"{d}/genesis.json"
cfg["Name"] = "lab-p27c"
cfg["Net"]["ListenPort"] = 58021
cfg["Net"]["BootstrapPeers"] = []
cfg["Net"]["MinConnectedPeers"] = 0
cfg["Net"]["MinPeers"] = 0
cfg["Net"]["P2PBackend"] = "legacy"
cfg["RPC"]["HTTPPort"] = 48021
json.dump(cfg, open(f"{d}/config.json", "w"), indent=4)
EOF
cd $M && setsid nohup nice -n 10 $W/build/znnd --data $M >> $M/boot.log 2>&1 < /dev/null &
sleep 15
cd ~/zenon-resilience-lab/results/phase-27-next-sporks
$P waitfor 8
$P sporkcreate p27c-x "phase27c unknown spork"
$P sporkid p27c-x | tee raw/p27c-spork-id.txt
stop_master

# ---------- C2: patch the COPY tree, rebuild, restart, activate ----------
XID=$(cat raw/p27c-spork-id.txt)
if [ -z "$XID" ]; then echo "p27c spork id missing — aborting"; exit 1; fi
cd $W
sed -i "s|BridgeAndLiquiditySpork = NewImplementedSpork(\"[0-9a-f]*\") // LAB p27 devnet spork|BridgeAndLiquiditySpork = NewImplementedSpork(\"$XID\") // LAB p27 devnet spork|" common/types/spork.go
grep "LAB p27" common/types/spork.go
make znnd >> /tmp/p27c-build.log 2>&1
echo "p27c build exit=$?"

cd $M && setsid nohup nice -n 10 $W/build/znnd --data $M >> $M/boot.log 2>&1 < /dev/null &
sleep 15
cd ~/zenon-resilience-lab/results/phase-27-next-sporks
$P sporkactivate $XID
ENF=$($P sporkwait p27c-x)
echo "p27c-x enforcement $ENF" | tee raw/p27c-enforcement.txt
$P waitfor $((ENF + 3))

# ---------- C3: v0.0.10 node ----------
ENODE=$(cd /tmp/enode && ./enode $M/network-private-key 58021)
echo "p27c master enode: $ENODE"
rm -rf $V
mkdir -p $V
python3 - "$V" "$ENODE" << 'EOF'
import json, sys
v, enode = sys.argv[1], sys.argv[2]
import shutil
shutil.copy("/home/mrstockholmez/zenon-resilience-lab/suite/devnet/genesis.json", f"{v}/genesis.json")
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

cd ~/zenon-resilience-lab/results/phase-27-next-sporks
MAXH=$((ENF + 30))
LASTV=0
EXIT_CODE=""
for i in $(seq 1 90); do
  if ! kill -0 $VPID 2>/dev/null; then
    wait $VPID; EXIT_CODE=$?
    break
  fi
  MH=$(FM); VHH=$(VH)
  if [ "$VHH" != "$LASTV" ]; then echo "t+$((i*5))s master=$MH v0010=$VHH enf=$ENF"; LASTV=$VHH; fi
  [ "$VHH" -ge "$MAXH" ] && break
  sleep 5
done
STILL=0
kill -0 $VPID 2>/dev/null && STILL=1
if [ $STILL -eq 1 ]; then kill $VPID 2>/dev/null; fi
[ -z "$EXIT_CODE" ] && EXIT_CODE="running_killed"
echo "v0010 still_running=$STILL exit_code=$EXIT_CODE lastHeight=$(VH) reachedPreEnforcement=$(( $(VH) >= ENF - 1 ? 1 : 0 ))"

grep -i 'spork\|terminat\|unimplemented\|fatal\|panic' $V/boot.log $V/log/*/*.log 2>/dev/null | tail -12 | tee raw/p27c-v0010-loglines.txt

python3 - "$ENF" "$EXIT_CODE" "$STILL" "$(VH)" << 'EOF' | tee raw/p27c-v0010-result.json
import json, sys
enf, code, still, lastv = sys.argv[1], sys.argv[2], int(sys.argv[3]), int(sys.argv[4])
reached = lastv >= int(enf) - 1
terminated = (still == 0 and code not in ("running_killed", ""))
print(json.dumps({
  "check": "c2-v0010-clean-exit-at-unknown-spork",
  "unknownSpork": "p27c-x",
  "enforcementHeight": int(enf),
  "v0010LastHeight": lastv,
  "reachedPreEnforcement": reached,
  "processTerminated": terminated,
  "exitCode": code,
  "expectedExitCode": 2,
  "cleanExitAtUnknownSpork": terminated and str(code) == "2" and reached,
}, indent=2))
EOF

echo "P27C DONE"
