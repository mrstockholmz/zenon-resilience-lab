#!/bin/bash
# phase27 resume (2026-11-14 09:xx): continue on the running p27 devnet chain
# (height ~880, sporks p27-bridge/p27-refund created and patched into the build,
# account 3 fused). Does: b1/b2 ack probes, bridge activation, governance,
# unburnable token + Owned pair, wrap #1 stall evidence, refund-spork activation,
# recovery evidence, wrap #2, b3/b4 recency probes.
set -u
cd ~/zenon-resilience-lab/results/phase-27-next-sporks
P=probe/phase27probe
LOG=raw/resume.log
exec >> "$LOG" 2>&1
set -x

BRIDGE_ID=$(cat raw/spork-bridge-id.txt)
REFUND_ID=$(cat raw/spork-refund-id.txt)

FM() { curl -s -m 5 -X POST http://127.0.0.1:48001 -H 'Content-Type: application/json' -d '{"jsonrpc":"2.0","id":1,"method":"ledger.getFrontierMomentum","params":[]}' | python3 -c "import json,sys; print(json.load(sys.stdin)['result']['height'])"; }

# ---- fund account 4 (ack-probe account) ----
$P send 3 4 ZNN 1000000000
$P recvall 4

# ---- b1/b2: DP enforced at 30; ack(29) trips older-than-previous ----
$P staleack 4 5 fresh fresh-ack-control | tee raw/fresh-ack-control.json
$P staleack 4 5 stale stale-ack-h29 29 | tee raw/stale-ack-h29.json

# ---- activate bridge spork ----
$P sporkactivate $BRIDGE_ID
ENF_B=$($P sporkwait p27-bridge)
echo "bridge enforcement $ENF_B"
$P waitfor $((ENF_B + 8))

# ---- bridge governance (accelerated delays: admin 20, soft 10) ----
$P bridge-orch 6 3 15 10
$P bridge-guardians
$P waitfor $(( $(FM) + 23 ))
$P bridge-guardians
$P bridge-tss "AsAQx1M3LVXCuozDOqO5b9adj/PItYgwZFG/xTDBiZzT"
$P waitfor $(( $(FM) + 13 ))
$P bridge-tss "AsAQx1M3LVXCuozDOqO5b9adj/PItYgwZFG/xTDBiZzT"
$P bridge-network 2 123 "Ethereum" "0x323b5d4c32345ced77393b3530b1eed0f346429d" "{}"

# ---- unburnable token + Owned pair registration ----
$P issue 3 unburnable UNBURN 100000 1000000 1 true false | tee raw/issue-unburnable.txt
ZTS=$(grep lastOwnedZts raw/issue-unburnable.txt | awk '{print $2}')
echo "ZTS=$ZTS" | tee raw/zts.txt
$P bridge-pair 2 123 "$ZTS" "0x5fbdb2315678afecb367f032d93f642f64180aa3" true true true 100 15 20 "{}"
$P waitfor $(( $(FM) + 13 ))
$P bridge-pair 2 123 "$ZTS" "0x5fbdb2315678afecb367f032d93f642f64180aa3" true true true 100 15 20 "{}"

# ---- WRAP #1 (pre-EmbeddedRefundSpork): Bridge Burn must fail + stall queue ----
echo CANARY5 > raw/canary-symbol.txt
$P wrap 3 "$ZTS" 10000 2 123 "0xb794f5ea0ba39494ce839613fffba74279579268" | tee raw/wrap-1.txt
sleep 60
$P refundstate "$ZTS" CANARY5 | tee raw/refundstate-0-stalled.json
$P issue 3 canary5 CANARY5 100 100 1 false true false | tee raw/canary-issue.txt
sleep 90
$P refundstate "$ZTS" CANARY5 | tee raw/refundstate-0b-canary-blocked.json

# ---- activate refund spork: queue must recover ----
$P sporkactivate $REFUND_ID
ENF_R=$($P sporkwait p27-refund)
echo "refund enforcement $ENF_R" | tee raw/refund-enforcement.txt
$P waitfor $((ENF_R + 4))
$P refundstate "$ZTS" CANARY5 | tee raw/refundstate-1-recovered.json

# ---- WRAP #2 (post-spork): no stall ----
$P wrap 3 "$ZTS" 10000 2 123 "0xb794f5ea0ba39494ce839613fffba74279579268" | tee raw/wrap-2.txt
sleep 60
$P refundstate "$ZTS" CANARY5 | tee raw/refundstate-2-wrap2.json

# ---- b3/b4: isolated #280 recency-rule probe ----
$P staleack 4 5 stale stale-ack-enf-minus-1 $((ENF_R - 1)) | tee raw/stale-ack-enf-minus-1.json
$P staleack 4 5 fresh fresh-ack-after-refund-spork | tee raw/fresh-ack-after-refund-spork.json
$P recvall 4

echo "RESUME DONE"
