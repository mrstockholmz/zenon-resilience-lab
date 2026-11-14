#!/bin/bash
# phase27 part 2: activate EmbeddedRefundSpork, observe stall recovery,
# post-spork wrap, and the isolated recency-rule admission probe.
set -u
cd ~/zenon-resilience-lab/results/phase-27-next-sporks
P=probe/phase27probe
LOG=raw/part2.log
exec >> "$LOG" 2>&1
set -x

ZTS=$(grep -h lastOwnedZts raw/issue-unburnable.txt | awk "{print $2}")
CANARY=$(cat raw/canary-symbol.txt)

# 0. state before activation (stall should be visible)
$P refundstate "$ZTS" "$CANARY" | tee raw/refundstate-0-stalled.json

# 1. activate the embedded-refund spork
$P sporkactivate 6281c65e3dcfd761e03db2c80dc70a2dc8d42a1f2f8a107e00d2c50c9796d8b4
ENF=$($P sporkwait p27-refund)
echo "refund spork enforcement height: $ENF" | tee raw/refund-enforcement.txt
$P waitfor $((ENF + 4))

# 2. stalled queue must have recovered: Burn receive finalized (failed),
#    refund deposited back on the bridge, canary Issue executed
$P refundstate "$ZTS" "$CANARY" | tee raw/refundstate-1-recovered.json

# 3. WRAP #2 (post-spork): refund must be immediate, no stall
$P wrap 3 "$ZTS" 10000 2 123 "0xb794f5ea0ba39494ce839613fffba74279579268" | tee raw/wrap-2.txt
sleep 60
$P refundstate "$ZTS" "$CANARY" | tee raw/refundstate-2-wrap2.json

# 4. isolated recency-rule admission probe:
#    account 4's previous block acked a pre-enforcement momentum, so acking
#    ENF-1 trips ONLY the #280 recency rule (not the older-than-previous rule)
$P staleack 4 5 stale stale-ack-enf-minus-1 $((ENF - 1)) | tee raw/stale-ack-enf-minus-1.json
$P staleack 4 5 fresh fresh-ack-after-refund-spork | tee raw/fresh-ack-after-refund-spork.json
$P recvall 4

echo "PART2 DONE"
