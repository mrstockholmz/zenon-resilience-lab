#!/bin/bash
# phase27 resume part 2 (2026-11-14 ~10:20): continue after probe receive-race.
# Governance done (orch/guardians x2/tss x2/network), unburnable UNBURN issued.
set -u
cd ~/zenon-resilience-lab/results/phase-27-next-sporks
P=probe/phase27probe
LOG=raw/resume2.log
exec >> "$LOG" 2>&1
set -x

ZTS=zts1axafdyzcsam59vszpwqtmq
echo "ZTS=$ZTS" | tee raw/zts.txt
REFUND_ID=$(cat raw/spork-refund-id.txt)

FM() { curl -s -m 5 -X POST http://127.0.0.1:48001 -H 'Content-Type: application/json' -d '{"jsonrpc":"2.0","id":1,"method":"ledger.getFrontierMomentum","params":[]}' | python3 -c "import json,sys; print(json.load(sys.stdin)['result']['height'])"; }

# Owned pair registration (soft-delay challenge: two calls ~13 apart)
$P bridge-pair 2 123 "$ZTS" "0x5fbdb2315678afecb367f032d93f642f64180aa3" true true true 100 15 20 "{}"
$P waitfor $(( $(FM) + 13 ))
$P bridge-pair 2 123 "$ZTS" "0x5fbdb2315678afecb367f032d93f642f64180aa3" true true true 100 15 20 "{}"

# WRAP #1 (pre-EmbeddedRefundSpork): Burn must fail + stall the token queue
echo CANARY5 > raw/canary-symbol.txt
$P wrap 3 "$ZTS" 10000 2 123 "0xb794f5ea0ba39494ce839613fffba74279579268" | tee raw/wrap-1.txt
sleep 75
$P refundstate "$ZTS" CANARY5 | tee raw/refundstate-0-stalled.json
$P issue 3 canary5 CANARY5 100 100 1 false true false | tee raw/canary-issue.txt
sleep 100
$P refundstate "$ZTS" CANARY5 | tee raw/refundstate-0b-canary-blocked.json

# activate refund spork: queue must recover
$P sporkactivate $REFUND_ID
ENF_R=$($P sporkwait p27-refund)
echo "refund enforcement $ENF_R" | tee raw/refund-enforcement.txt
$P waitfor $((ENF_R + 4))
$P refundstate "$ZTS" CANARY5 | tee raw/refundstate-1-recovered.json

# WRAP #2 (post-spork): no stall
$P wrap 3 "$ZTS" 10000 2 123 "0xb794f5ea0ba39494ce839613fffba74279579268" | tee raw/wrap-2.txt
sleep 75
$P refundstate "$ZTS" CANARY5 | tee raw/refundstate-2-wrap2.json

# b3/b4: isolated #280 recency probe (account 4's frontier block acked 896 < ENF_R)
$P staleack 4 5 stale stale-ack-enf-minus-1 $((ENF_R - 1)) | tee raw/stale-ack-enf-minus-1.json
$P staleack 4 5 fresh fresh-ack-after-refund-spork | tee raw/fresh-ack-after-refund-spork.json
$P recvall 4

echo "RESUME2 DONE"
