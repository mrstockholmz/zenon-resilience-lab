#!/bin/bash
# phase27 chain-C (p27c): demonstrate the CLEAN old-node behavior — v0.0.10
# syncs from a master node up to the first unknown-spork enforcement and
# terminates (vs chain 1/p27 where the DP wire change blocks sync entirely).
# Genesis: libp2p spork @4000, dynamic-plasma @5000 (beyond the test window),
# so pre-enforcement momentums keep the v0.0.10-decodable wire form.
# Requires the build to carry THIS chain's spork ids (patched before launch).
set -u
cd ~/zenon-resilience-lab/results/phase-27-next-sporks
export LAB_RPC_URL=http://127.0.0.1:48021
P=probe/phase27probe
LOG=raw/p27c.log
exec >> "$LOG" 2>&1
set -x

FM() { curl -s -m 5 -X POST http://127.0.0.1:48021 -H 'Content-Type: application/json' -d '{"jsonrpc":"2.0","id":1,"method":"ledger.getFrontierMomentum","params":[]}' | python3 -c "import json,sys; print(json.load(sys.stdin)['result']['height'])"; }
start_node() { cd ~/lab-devnet-p27c && setsid nohup nice -n 10 ~/lab-go-zenon-master/build/znnd --data ~/lab-devnet-p27c >> ~/lab-devnet-p27c/boot.log 2>&1 < /dev/null & sleep 15; }
stop_node() { pkill -f "lab-devnet-p27c" || true; sleep 6; }

D=~/lab-devnet-p27c
stop_node
rm -rf $D
mkdir -p $D/wallet
python3 - "$D" << 'EOF'
import json, sys
d = sys.argv[1]
g = json.load(open("/home/mrstockholmez/zenon-resilience-lab/suite/devnet/genesis.json"))
# push the two v0.0.10-unknown sporks beyond the test window
for s in g.get("SporkConfig", {}).get("Sporks", []):
    if s.get("Name") == "libp2p":
        s["EnforcementHeight"] = 4000
    if s.get("Name") == "dynamic-plasma":
        s["EnforcementHeight"] = 5000
json.dump(g, open(f"{d}/genesis.json", "w"), indent=4)
p = None
import shutil
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
print("p27c genesis+config written")
EOF
start_node
$P waitfor 8
$P sporkcreate p27-bridge "phase27c bridge activation"
$P sporkid p27-bridge | tee raw/p27c-spork-bridge-id.txt
stop_node
BRIDGE_ID=$(cat raw/p27c-spork-bridge-id.txt)
echo "p27c bridge spork id: $BRIDGE_ID"
