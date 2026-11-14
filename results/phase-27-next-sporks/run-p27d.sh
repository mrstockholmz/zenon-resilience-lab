#!/bin/bash
# phase27 chain-D (p27d): corrected re-run of the p27b flow. p27b botched the
# (a) test because wrap #1 raced the UNBURN mint receive (published before the
# mint was ledger-visible -> "insufficient balance", tee swallowed the FATAL)
# and the (b) fresh-ack controls raced account 4's funding receive. Here:
#   - after issuing UNBURN, poll until account 3 actually holds the mint
#   - recvall 4 (with retries) before the ack probes
#   - all wrap/issue output captured with 2>&1 | tee
# Chain: fresh ~/lab-devnet-p27d, RPC 48031, net 58031.
# Artifacts overwrite the canonical raw/ names the checker reads; the broken
# p27b copies were moved to raw/failed-p27b/ before this run.
set -u
cd ~/zenon-resilience-lab/results/phase-27-next-sporks
export LAB_RPC_URL=http://127.0.0.1:48031
P=probe/phase27probe
LOG=raw/p27d.log
exec >> "$LOG" 2>&1
set -x

FM() { curl -s -m 5 -X POST http://127.0.0.1:48031 -H 'Content-Type: application/json' -d '{"jsonrpc":"2.0","id":1,"method":"ledger.getFrontierMomentum","params":[]}' | python3 -c "import json,sys; print(json.load(sys.stdin)['result']['height'])"; }
ZTSBAL() { curl -s -m 5 -X POST http://127.0.0.1:48031 -H 'Content-Type: application/json' -d "{\"jsonrpc\":\"2.0\",\"id\":1,\"method\":\"ledger.getAccountInfoByAddress\",\"params\":[\"$1\"]}" | python3 -c "import json,sys; m=json.load(sys.stdin)['result']['balanceInfoMap']; print(m.get('$2',{}).get('balance','0'))" 2>/dev/null || echo 0; }
start_node() { cd ~/lab-devnet-p27d && setsid nohup nice -n 10 ~/lab-go-zenon-master/build/znnd --data ~/lab-devnet-p27d >> ~/lab-devnet-p27d/boot.log 2>&1 < /dev/null & sleep 15; }
stop_node() { PID=$(ps aux | grep 'lab-devnet-p27[d]' | awk '{print $2}'); [ -n "$PID" ] && kill $PID; sleep 6; }

# ---------- D0: fresh chain, fund, create sporks ----------
stop_node
D=~/lab-devnet-p27d
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
cfg["Name"] = "lab-p27d"
cfg["Net"]["ListenPort"] = 58031
cfg["Net"]["BootstrapPeers"] = []
cfg["Net"]["MinConnectedPeers"] = 0
cfg["Net"]["MinPeers"] = 0
cfg["RPC"]["HTTPPort"] = 48031
json.dump(cfg, open(f"{d}/config.json", "w"), indent=4)
EOF
start_node
$P waitfor 8
$P send 3 0 QSR 1500000000000
$P send 3 0 ZNN 500000000
$P fuse 3 10000000000
$P send 3 4 ZNN 1000000000
$P sporkcreate p27-bridge "phase27 bridge activation"
$P sporkid p27-bridge | tee raw/p27d-spork-bridge-id.txt
$P sporkcreate p27-refund "phase27 embedded refund"
$P sporkid p27-refund | tee raw/p27d-spork-refund-id.txt
stop_node

# ---------- D1: patch real ids, rebuild ----------
BRIDGE_ID=$(cat raw/p27d-spork-bridge-id.txt)
REFUND_ID=$(cat raw/p27d-spork-refund-id.txt)
if [ -z "$BRIDGE_ID" ] || [ -z "$REFUND_ID" ]; then
  echo "p27d spork ids missing — aborting"
  exit 1
fi
cd ~/lab-go-zenon-master
sed -i "s|BridgeAndLiquiditySpork = NewImplementedSpork(\"[0-9a-f]*\") // LAB p27 devnet spork|BridgeAndLiquiditySpork = NewImplementedSpork(\"$BRIDGE_ID\") // LAB p27 devnet spork|" common/types/spork.go
sed -i "s|EmbeddedRefundSpork = NewImplementedSpork(\"[0-9a-f]*\") // LAB p27 devnet spork|EmbeddedRefundSpork = NewImplementedSpork(\"$REFUND_ID\") // LAB p27 devnet spork|" common/types/spork.go
grep "LAB p27 devnet spork" common/types/spork.go
make znnd >> /tmp/p27d-build.log 2>&1
echo "p27d build exit=$?"

# ---------- D2: activate bridge, governance ----------
cd ~/zenon-resilience-lab/results/phase-27-next-sporks
start_node
$P sporkactivate $BRIDGE_ID
ENF_B=$($P sporkwait p27-bridge)
echo "p27d bridge enforcement $ENF_B" | tee raw/p27d-bridge-enforcement.txt
$P waitfor $((ENF_B + 6))

$P bridge-orch 6 3 15 10
$P bridge-guardians
$P waitfor $(( $(FM) + 24 ))
$P bridge-guardians
$P bridge-tss "AsAQx1M3LVXCuozDOqO5b9adj/PItYgwZFG/xTDBiZzT"
$P waitfor $(( $(FM) + 14 ))
$P bridge-tss "AsAQx1M3LVXCuozDOqO5b9adj/PItYgwZFG/xTDBiZzT"
$P bridge-network 2 123 "Ethereum" "0x323b5d4c32345ced77393b3530b1eed0f346429d" "{}"

# ---------- D3: unburnable token + Owned pair (respect soft delay) ----------
$P issue 3 unburnable UNBURN 100000 1000000 1 true false 2>&1 | tee raw/issue-unburnable.txt
ZTS=$(grep lastOwnedZts raw/issue-unburnable.txt | awk '{print $2}')
if [ -z "$ZTS" ]; then
  sleep 30; $P recvall 3
  ZTS=$(curl -s -m 5 -X POST http://127.0.0.1:48031 -H 'Content-Type: application/json' -d '{"jsonrpc":"2.0","id":1,"method":"embedded.token.getByOwner","params":["z1qp3yph55qgresyytz83anynr2f4z39x2z3ej3e",0,10]}' | python3 -c "import json,sys; l=json.load(sys.stdin)['result']['list']; print(l[-1]['tokenStandard'] if l else '')")
fi
echo "ZTS=$ZTS" | tee raw/zts.txt

# --- FIX 1: do not wrap until the mint is actually ledger-visible ---
A3=$($P addrs | awk '$1==3{print $2}')
for i in $(seq 1 30); do
  BAL=$(ZTSBAL "$A3" "$ZTS")
  echo "p27d account3 UNBURN balance poll $i: $BAL"
  [ "$BAL" != "0" ] && break
  $P recvall 3; sleep 10
done

$P bridge-pair 2 123 "$ZTS" "0x5fbdb2315678afecb367f032d93f642f64180aa3" true true true 100 15 20 "{}"
$P waitfor $(( $(FM) + 14 ))
$P bridge-pair 2 123 "$ZTS" "0x5fbdb2315678afecb367f032d93f642f64180aa3" true true true 100 15 20 "{}"
$P waitfor $(( $(FM) + 4 ))

# --- FIX 2: account 4 must hold the funding before the ack probes ---
A4=$($P addrs | awk '$1==4{print $2}')
for i in $(seq 1 20); do
  BAL4=$(ZTSBAL "$A4" "zts1znnxxxxxxxxxxxxx9z4ulx")
  echo "p27d account4 ZNN balance poll $i: $BAL4"
  [ "$BAL4" != "0" ] && break
  $P recvall 4; sleep 10
done

# ---------- D4: b1/b2 ack probes (DP enforced at 30) ----------
$P staleack 4 5 fresh fresh-ack-control 2>&1 | tee raw/fresh-ack-control.json
$P staleack 4 5 stale stale-ack-h29 29 2>&1 | tee raw/stale-ack-h29.json

# ---------- D5: WRAP #1 pre-refund-spork: Burn fails, queue stalls ----------
echo CANARY5 > raw/canary-symbol.txt
$P wrap 3 "$ZTS" 10000 2 123 "0xb794f5ea0ba39494ce839613fffba74279579268" 2>&1 | tee raw/wrap-1.txt
$P waitfor $(( $(FM) + 4 ))
$P refundstate "$ZTS" CANARY5 | tee raw/refundstate-0-stalled.json
$P issue 3 canary5 CANARY5 100 100 1 false true false 2>&1 | tee raw/canary-issue.txt
$P waitfor $(( $(FM) + 4 ))
$P refundstate "$ZTS" CANARY5 | tee raw/refundstate-0b-canary-blocked.json

# ---------- D6: activate refund spork: queue recovers ----------
$P sporkactivate $REFUND_ID
ENF_R=$($P sporkwait p27-refund)
echo "refund enforcement $ENF_R" | tee raw/refund-enforcement.txt
$P waitfor $((ENF_R + 4))
$P refundstate "$ZTS" CANARY5 | tee raw/refundstate-1-recovered.json

# ---------- D7: WRAP #2 post-spork: no stall ----------
$P wrap 3 "$ZTS" 10000 2 123 "0xb794f5ea0ba39494ce839613fffba74279579268" 2>&1 | tee raw/wrap-2.txt
$P waitfor $(( $(FM) + 4 ))
$P refundstate "$ZTS" CANARY5 | tee raw/refundstate-2-wrap2.json

# ---------- D8: b3/b4 isolated recency probe ---
$P staleack 4 5 stale stale-ack-enf-minus-1 $((ENF_R - 1)) 2>&1 | tee raw/stale-ack-enf-minus-1.json
$P staleack 4 5 fresh fresh-ack-after-refund-spork 2>&1 | tee raw/fresh-ack-after-refund-spork.json
$P recvall 4

echo "P27D DONE"
