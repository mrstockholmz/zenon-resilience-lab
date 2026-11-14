#!/bin/bash
# phase27 chain-B (p27b): full fresh reproduction of the EmbeddedRefundSpork
# scenario on a new isolated devnet (~/lab-devnet-p27b, RPC 48011, net 58011).
# Chain 1 (p27) botched the pre-spork window (pair confirm raced the soft
# delay), so the a-checks are redone here from genesis, end to end.
# Assumes run after part3; the p27 node may keep running (its build image is
# in memory; the rebuild below only affects future starts).
set -u
cd ~/zenon-resilience-lab/results/phase-27-next-sporks
export LAB_RPC_URL=http://127.0.0.1:48011
P=probe/phase27probe
LOG=raw/p27b.log
exec >> "$LOG" 2>&1
set -x

FM() { curl -s -m 5 -X POST http://127.0.0.1:48011 -H 'Content-Type: application/json' -d '{"jsonrpc":"2.0","id":1,"method":"ledger.getFrontierMomentum","params":[]}' | python3 -c "import json,sys; print(json.load(sys.stdin)['result']['height'])"; }
start_node() { cd ~/lab-devnet-p27b && setsid nohup nice -n 10 ~/lab-go-zenon-master/build/znnd --data ~/lab-devnet-p27b >> ~/lab-devnet-p27b/boot.log 2>&1 < /dev/null & sleep 15; }
stop_node() { pkill -f "lab-devnet-p27b" || true; sleep 6; }

# ---------- B0: fresh chain, fund, create sporks ----------
stop_node
D=~/lab-devnet-p27b
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
cfg["Name"] = "lab-p27b"
cfg["Net"]["ListenPort"] = 58011
cfg["Net"]["BootstrapPeers"] = []
cfg["Net"]["MinConnectedPeers"] = 0
cfg["Net"]["MinPeers"] = 0
cfg["RPC"]["HTTPPort"] = 48011
json.dump(cfg, open(f"{d}/config.json", "w"), indent=4)
EOF
start_node
$P waitfor 8
$P send 3 0 QSR 1500000000000
$P send 3 0 ZNN 500000000
$P fuse 0 10000000000
$P fuse 3 10000000000
$P send 3 4 ZNN 1000000000
sleep 45
$P recvall 4
$P sporkcreate p27-bridge "phase27 bridge activation"
$P sporkid p27-bridge | tee raw/p27b-spork-bridge-id.txt
$P sporkcreate p27-refund "phase27 embedded refund"
$P sporkid p27-refund | tee raw/p27b-spork-refund-id.txt
stop_node

# ---------- B1: patch real ids, rebuild ----------
BRIDGE_ID=$(cat raw/p27b-spork-bridge-id.txt)
REFUND_ID=$(cat raw/p27b-spork-refund-id.txt)
if [ -z "$BRIDGE_ID" ] || [ -z "$REFUND_ID" ]; then
  echo "p27b spork ids missing — aborting"
  exit 1
fi
cd ~/lab-go-zenon-master
sed -i "s|BridgeAndLiquiditySpork = NewImplementedSpork(\"[0-9a-f]*\") // LAB p27 devnet spork|BridgeAndLiquiditySpork = NewImplementedSpork(\"$BRIDGE_ID\") // LAB p27 devnet spork|" common/types/spork.go
sed -i "s|EmbeddedRefundSpork = NewImplementedSpork(\"[0-9a-f]*\") // LAB p27 devnet spork|EmbeddedRefundSpork = NewImplementedSpork(\"$REFUND_ID\") // LAB p27 devnet spork|" common/types/spork.go
grep "LAB p27 devnet spork" common/types/spork.go
make znnd >> /tmp/p27b-build.log 2>&1
echo "p27b build exit=$?"

# ---------- B2: activate bridge, governance ----------
cd ~/zenon-resilience-lab/results/phase-27-next-sporks
start_node
$P sporkactivate $BRIDGE_ID
ENF_B=$($P sporkwait p27-bridge)
echo "p27b bridge enforcement $ENF_B" | tee raw/p27b-bridge-enforcement.txt
$P waitfor $((ENF_B + 6))

$P bridge-orch 6 3 15 10
$P bridge-guardians
$P waitfor $(( $(FM) + 24 ))
$P bridge-guardians
$P bridge-tss "AsAQx1M3LVXCuozDOqO5b9adj/PItYgwZFG/xTDBiZzT"
$P waitfor $(( $(FM) + 14 ))
$P bridge-tss "AsAQx1M3LVXCuozDOqO5b9adj/PItYgwZFG/xTDBiZzT"
$P bridge-network 2 123 "Ethereum" "0x323b5d4c32345ced77393b3530b1eed0f346429d" "{}"

# ---------- B3: unburnable token + Owned pair (respect soft delay) ----------
$P issue 3 unburnable UNBURN 100000 1000000 1 true false | tee raw/issue-unburnable.txt
ZTS=$(grep lastOwnedZts raw/issue-unburnable.txt | awk '{print $2}')
if [ -z "$ZTS" ]; then
  sleep 30; $P recvall 3
  ZTS=$(curl -s -m 5 -X POST http://127.0.0.1:48011 -H 'Content-Type: application/json' -d '{"jsonrpc":"2.0","id":1,"method":"embedded.token.getByOwner","params":["z1qp3yph55qgresyytz83anynr2f4z39x2z3ej3e",0,10]}' | python3 -c "import json,sys; l=json.load(sys.stdin)['result']['list']; print(l[-1]['tokenStandard'] if l else '')")
fi
echo "ZTS=$ZTS" | tee raw/zts.txt

$P bridge-pair 2 123 "$ZTS" "0x5fbdb2315678afecb367f032d93f642f64180aa3" true true true 100 15 20 "{}"
$P waitfor $(( $(FM) + 14 ))
$P bridge-pair 2 123 "$ZTS" "0x5fbdb2315678afecb367f032d93f642f64180aa3" true true true 100 15 20 "{}"

# ---------- B4: b1/b2 ack probes (DP enforced at 30) ----------
$P staleack 4 5 fresh fresh-ack-control | tee raw/fresh-ack-control.json
$P staleack 4 5 stale stale-ack-h29 29 | tee raw/stale-ack-h29.json

# ---------- B5: WRAP #1 pre-refund-spork: Burn fails, queue stalls ----------
echo CANARY5 > raw/canary-symbol.txt
$P wrap 3 "$ZTS" 10000 2 123 "0xb794f5ea0ba39494ce839613fffba74279579268" | tee raw/wrap-1.txt
$P waitfor $(( $(FM) + 4 ))
$P refundstate "$ZTS" CANARY5 | tee raw/refundstate-0-stalled.json
$P issue 3 canary5 CANARY5 100 100 1 false true false | tee raw/canary-issue.txt
$P waitfor $(( $(FM) + 4 ))
$P refundstate "$ZTS" CANARY5 | tee raw/refundstate-0b-canary-blocked.json

# ---------- B6: activate refund spork: queue recovers ----------
$P sporkactivate $REFUND_ID
ENF_R=$($P sporkwait p27-refund)
echo "refund enforcement $ENF_R" | tee raw/refund-enforcement.txt
$P waitfor $((ENF_R + 4))
$P refundstate "$ZTS" CANARY5 | tee raw/refundstate-1-recovered.json

# ---------- B7: WRAP #2 post-spork: no stall ----------
$P wrap 3 "$ZTS" 10000 2 123 "0xb794f5ea0ba39494ce839613fffba74279579268" | tee raw/wrap-2.txt
$P waitfor $(( $(FM) + 4 ))
$P refundstate "$ZTS" CANARY5 | tee raw/refundstate-2-wrap2.json

# ---------- B8: b3/b4 isolated recency probe ----------
$P staleack 4 5 stale stale-ack-enf-minus-1 $((ENF_R - 1)) | tee raw/stale-ack-enf-minus-1.json
$P staleack 4 5 fresh fresh-ack-after-refund-spork | tee raw/fresh-ack-after-refund-spork.json
$P recvall 4

echo "P27B DONE"
