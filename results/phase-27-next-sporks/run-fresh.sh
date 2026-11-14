#!/bin/bash
# phase27 fresh end-to-end run.
# Build A (unhalt=5 patch, placeholder spork ids) must already be built.
set -u
cd ~/zenon-resilience-lab/results/phase-27-next-sporks
P=probe/phase27probe
LOG=raw/fresh.log
exec >> "$LOG" 2>&1
set -x

start_node() { # $1 = binary
  cd ~/lab-devnet-p27 && setsid nohup nice -n 10 "$1" --data ~/lab-devnet-p27 >> ~/lab-devnet-p27/boot.log 2>&1 < /dev/null &
  sleep 20
  curl -s -m 5 -X POST http://127.0.0.1:48001 -H 'Content-Type: application/json' -d '{"jsonrpc":"2.0","id":1,"method":"ledger.getFrontierMomentum","params":[]}' | python3 -c "import json,sys; print('boot frontier', json.load(sys.stdin)['result']['height'])"
}
stop_node() {
  pkill -f "znnd --data /home/mrstockholmez/lab-devnet-p27" || true
  pkill -f "build/znnd --data /home/mrstockholmez/lab-devnet-p27" || true
  sleep 6
}

# ---------- F1: fresh chain, create sporks on build A ----------
stop_node
D=~/lab-devnet-p27
rm -rf $D
mkdir -p $D/wallet
cp ~/zenon-resilience-lab/suite/devnet/genesis.json $D/genesis.json
cp ~/go-zenon/docker/devnet/pillar/wallet/z1qq9n7fpaqd8lpcljandzmx4xtku9w4ftwyg0mq $D/wallet/
python3 - "$D" << 'EOF'
import json, sys
d = sys.argv[1]
p = f"{d}/wallet/z1qq9n7fpaqd8lpcljandzmx4xtku9w4ftwyg0mq"
k = json.load(open(p)); k["Path"] = p; json.dump(k, open(p, "w"), indent=4)
cfg = json.load(open("/home/mrstockholmez/lab-devnet/config.json"))
cfg["DataPath"] = d
cfg["GenesisFile"] = f"{d}/genesis.json"
cfg["Name"] = "lab-p27"
cfg["Net"]["ListenPort"] = 58001
cfg["Net"]["BootstrapPeers"] = []
cfg["Net"]["MinConnectedPeers"] = 0
cfg["Net"]["MinPeers"] = 0
cfg["RPC"]["HTTPPort"] = 48001
json.dump(cfg, open(f"{d}/config.json", "w"), indent=4)
EOF
start_node ~/lab-go-zenon-master/build/znnd
$P waitfor 15

# fund index 0 and index 3 with QSR for plasma (kill the PoW waits)
$P send 3 0 QSR 1500000000000   # 15,000 QSR? no: 150e8 = 150 QSR
$P send 3 0 ZNN 500000000        # 5 ZNN
$P fuse 0 10000000000            # fuse 100 QSR for index 0 (10 txs/momentum)
$P fuse 3 10000000000            # fuse 100 QSR for index 3
sleep 40

# create both sporks and record their ids
$P sporkcreate p27-bridge "phase27 bridge activation"
$P sporkid p27-bridge | tee raw/spork-bridge-id.txt
$P sporkcreate p27-refund "phase27 embedded refund"
$P sporkid p27-refund | tee raw/spork-refund-id.txt
stop_node

# ---------- build B: patch the real spork ids ----------
BRIDGE_ID=$(cat raw/spork-bridge-id.txt)
REFUND_ID=$(cat raw/spork-refund-id.txt)
if [ -z "$BRIDGE_ID" ] || [ -z "$REFUND_ID" ]; then
  echo "spork ids missing — aborting before build B"
  exit 1
fi
cd ~/lab-go-zenon-master
sed -i "s|BridgeAndLiquiditySpork = NewImplementedSpork(\"[0-9a-f]*\") // LAB p27 devnet spork|BridgeAndLiquiditySpork = NewImplementedSpork(\"$BRIDGE_ID\") // LAB p27 devnet spork|" common/types/spork.go
sed -i "s|EmbeddedRefundSpork = NewImplementedSpork(\"[0-9a-f]*\") // LAB p27 devnet spork|EmbeddedRefundSpork = NewImplementedSpork(\"$REFUND_ID\") // LAB p27 devnet spork|" common/types/spork.go
grep "LAB p27 devnet spork" common/types/spork.go
make znnd >> /tmp/build-B.log 2>&1
echo "build B exit=$?"

# ---------- F2: governance + refund scenario on build B ----------
start_node ~/lab-go-zenon-master/build/znnd
$P sporkactivate $BRIDGE_ID
ENF_B=$($P sporkwait p27-bridge)
echo "bridge enforcement $ENF_B"
$P waitfor $((ENF_B + 8))   # + unhalt window (5) margin

$P bridge-orch 6 3 15 10
$P bridge-guardians
FM=$(curl -s -m 5 -X POST http://127.0.0.1:48001 -H 'Content-Type: application/json' -d '{"jsonrpc":"2.0","id":1,"method":"ledger.getFrontierMomentum","params":[]}' | python3 -c "import json,sys; print(json.load(sys.stdin)['result']['height'])")
$P waitfor $((FM + 23))
$P bridge-guardians
$P bridge-tss "AsAQx1M3LVXCuozDOqO5b9adj/PItYgwZFG/xTDBiZzT"
FM=$(curl -s -m 5 -X POST http://127.0.0.1:48001 -H 'Content-Type: application/json' -d '{"jsonrpc":"2.0","id":1,"method":"ledger.getFrontierMomentum","params":[]}' | python3 -c "import json,sys; print(json.load(sys.stdin)['result']['height'])")
$P waitfor $((FM + 13))
$P bridge-tss "AsAQx1M3LVXCuozDOqO5b9adj/PItYgwZFG/xTDBiZzT"
$P bridge-network 2 123 "Ethereum" "0x323b5d4c32345ced77393b3530b1eed0f346429d" "{}"
$P issue 3 unburnable UNBURN 100000 1000000 1 true false | tee raw/issue-unburnable.txt
ZTS=$(grep lastOwnedZts raw/issue-unburnable.txt | awk '{print $2}')
echo "ZTS=$ZTS"
$P bridge-pair 2 123 "$ZTS" "0x5fbdb2315678afecb367f032d93f642f64180aa3" true true true 100 15 20 "{}"
FM=$(curl -s -m 5 -X POST http://127.0.0.1:48001 -H 'Content-Type: application/json' -d '{"jsonrpc":"2.0","id":1,"method":"ledger.getFrontierMomentum","params":[]}' | python3 -c "import json,sys; print(json.load(sys.stdin)['result']['height'])")
$P waitfor $((FM + 13))
$P bridge-pair 2 123 "$ZTS" "0x5fbdb2315678afecb367f032d93f642f64180aa3" true true true 100 15 20 "{}"

echo CANARY5 > raw/canary-symbol.txt
# wrap #1 (pre-refund-spork): must stall
$P wrap 3 "$ZTS" 10000 2 123 "0xb794f5ea0ba39494ce839613fffba74279579268" | tee raw/wrap-1.txt
sleep 60
$P refundstate "$ZTS" CANARY5 | tee raw/refundstate-0-stalled.json
$P issue 3 canary5 CANARY5 100 100 1 false true false | tee raw/canary5-issue.txt
sleep 60
$P refundstate "$ZTS" CANARY5 | tee raw/refundstate-0b-canary-blocked.json

# activate the refund spork: queue must recover
$P sporkactivate $REFUND_ID
ENF_R=$($P sporkwait p27-refund)
echo "refund enforcement $ENF_R" | tee raw/refund-enforcement.txt
$P waitfor $((ENF_R + 4))
$P refundstate "$ZTS" CANARY5 | tee raw/refundstate-1-recovered.json

# wrap #2 (post-spork): no stall
$P wrap 3 "$ZTS" 10000 2 123 "0xb794f5ea0ba39494ce839613fffba74279579268" | tee raw/wrap-2.txt
sleep 60
$P refundstate "$ZTS" CANARY5 | tee raw/refundstate-2-wrap2.json

# isolated recency-rule probe
$P staleack 4 5 stale stale-ack-enf-minus-1 $((ENF_R - 1)) | tee raw/stale-ack-enf-minus-1.json
$P staleack 4 5 fresh fresh-ack-after-refund-spork | tee raw/fresh-ack-after-refund-spork.json

echo "FRESH DONE"
