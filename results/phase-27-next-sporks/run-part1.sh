#!/bin/bash
# phase27 part 1: bridge governance + pre-refund-spork wrap (#271 live trigger).
# Runs against the isolated p27 devnet (master, patched spork ids + accelerated delays).
set -u
cd ~/zenon-resilience-lab/results/phase-27-next-sporks
P=probe/phase27probe
LOG=raw/part1.log
exec >> "$LOG" 2>&1
set -x

# 1. wait for the bridge spork to be enforced (+3 momentums margin)
ENF=$($P sporkwait p27-bridge)
echo "bridge spork enforcement height: $ENF"
$P waitfor $((ENF + 3))

# 2. orchestrator info (immediate)
$P bridge-orch 6 3 15 10

# 3. TSS key change via administrator soft-delay challenge (10 momentums, two calls)
$P bridge-tss "AsAQx1M3LVXCuozDOqO5b9adj/PItYgwZFG/xTDBiZzT"
FM=$(curl -s -m 5 -X POST http://127.0.0.1:48001 -H 'Content-Type: application/json' -d '{"jsonrpc":"2.0","id":1,"method":"ledger.getFrontierMomentum","params":[]}' | python3 -c "import json,sys; print(json.load(sys.stdin)['result']['height'])")
$P waitfor $((FM + 12))
$P bridge-tss "AsAQx1M3LVXCuozDOqO5b9adj/PItYgwZFG/xTDBiZzT"

# 4. register network (immediate)
$P bridge-network 2 123 "Ethereum" "0x323b5d4c32345ced77393b3530b1eed0f346429d" "{}"

# 5. issue an unburnable, mintable token from account 3 and receive it
$P issue 3 unburnable UNBURN 100000 1000000 1 true false | tee raw/issue-unburnable.txt
ZTS=$(grep lastOwnedZts raw/issue-unburnable.txt | awk '{print $2}')
echo "ZTS=$ZTS" | tee raw/zts.txt

# 6. register it as an Owned pair (soft-delay challenge: two calls, 10 momentums apart)
$P bridge-pair 2 123 "$ZTS" "0x5fbdb2315678afecb367f032d93f642f64180aa3" true true true 100 15 20 "{}"
FM=$(curl -s -m 5 -X POST http://127.0.0.1:48001 -H 'Content-Type: application/json' -d '{"jsonrpc":"2.0","id":1,"method":"ledger.getFrontierMomentum","params":[]}' | python3 -c "import json,sys; print(json.load(sys.stdin)['result']['height'])")
$P waitfor $((FM + 12))
$P bridge-pair 2 123 "$ZTS" "0x5fbdb2315678afecb367f032d93f642f64180aa3" true true true 100 15 20 "{}"

# 7. observe pre-wrap state
$P observe pre-wrap-1 | tee raw/observe-pre-wrap-1.json

# 8. WRAP #1 (pre-EmbeddedRefundSpork): 10000 units -> Bridge Burn fails -> stall
$P wrap 3 "$ZTS" 10000 2 123 "0xb794f5ea0ba39494ce839613fffba74279579268" | tee raw/wrap-1.txt

# 9. wait for wrap execution + a few momentums, then observe the stall
sleep 60
$P observe post-wrap-1-stall | tee raw/observe-post-wrap-1.json

# 10. canary: an unrelated Issue queued behind the stalled Burn must NOT execute
$P issue 3 canary QUEUED 100 100 1 false true false | tee raw/canary-issue.txt
sleep 90
$P observe post-canary | tee raw/observe-post-canary.json

echo "PART1 DONE"
