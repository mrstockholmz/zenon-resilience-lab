#!/bin/bash
set -u
cd ~/zenon-resilience-lab/results/phase-27-next-sporks
P=probe/phase27probe
exec >> raw/repair.log 2>&1
set -x
$P waitfor 433
$P bridge-guardians
sleep 75
curl -s -m 5 -X POST http://127.0.0.1:48001 -H 'Content-Type: application/json' -d '{"jsonrpc":"2.0","id":1,"method":"embedded.bridge.getSecurityInfo","params":[]}' | python3 -c "import json,sys; d=json.load(sys.stdin)['result']; print('guardians:', len(d['guardians']))"
# TSS soft-delay challenge
$P bridge-tss "AsAQx1M3LVXCuozDOqO5b9adj/PItYgwZFG/xTDBiZzT"
FM=$(curl -s -m 5 -X POST http://127.0.0.1:48001 -H 'Content-Type: application/json' -d '{"jsonrpc":"2.0","id":1,"method":"ledger.getFrontierMomentum","params":[]}' | python3 -c "import json,sys; print(json.load(sys.stdin)['result']['height'])")
$P waitfor $((FM+13))
$P bridge-tss "AsAQx1M3LVXCuozDOqO5b9adj/PItYgwZFG/xTDBiZzT"
sleep 80
curl -s -m 5 -X POST http://127.0.0.1:48001 -H 'Content-Type: application/json' -d '{"jsonrpc":"2.0","id":1,"method":"embedded.bridge.getBridgeInfo","params":[]}' | python3 -c "import json,sys; d=json.load(sys.stdin)['result']; print('tss set:', d['compressedTssECDSAPubKey'] != '')"
# wrap #1 (pre-refund-spork) - burn should fail -> stall
$P wrap 3 zts1223q8jl79yamzynyjk2paf 10000 2 123 "0xb794f5ea0ba39494ce839613fffba74279579268" | tee raw/wrap-1.txt
sleep 90
$P refundstate zts1223q8jl79yamzynyjk2paf CANARY3 | tee raw/refundstate-0-stalled.json
$P issue 3 canary3 CANARY3 100 100 1 false true false | tee raw/canary3-issue.txt
sleep 100
$P refundstate zts1223q8jl79yamzynyjk2paf CANARY3 | tee raw/refundstate-0b-canary-blocked.json
echo "REPAIR DONE"
