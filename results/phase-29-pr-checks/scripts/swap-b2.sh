#!/bin/bash
# Phase 29 #306 drip A/B: swap the victim from the post-fix binary (A2) to the
# pre-fix control binary (B2) at a fixed time, unattended. The liar (ZNN_LIAR=2,
# ~/p29-liar/build/znnd-liar, data ~/lab-devnet-p29/s306f2) keeps running.
#
# Usage: nohup scripts/swap-b2.sh <HH:MM> >> raw/swap-b2.log 2>&1 &
set -u
WHEN="${1:?usage: swap-b2.sh HH:MM}"
echo "[swap-b2] armed, will swap at $WHEN CET; now $(date +%T)"
while [ "$(date +%H:%M)" != "$WHEN" ]; do sleep 20; done
echo "[swap-b2] $(date +%T) swapping victim to pre-fix binary (p29-pre = 65240b1)"

echo 'B2-prefix-dripv6' > ~/zenon-resilience-lab/results/phase-29-pr-checks/raw/liar-phase.txt

pkill -f 'review/pr-306/build/znnd --data /home/mrstockholmez/lab-devnet-p29/s306v' || true
sleep 8
pkill -9 -f 'lab-devnet-p29/s306v' || true
sleep 2

cd ~/lab-devnet-p29/s306v
setsid nohup nice -n 10 ~/p29-pre/build/znnd --data ~/lab-devnet-p29/s306v \
  > ~/lab-devnet-p29/s306v/boot-b2.log 2>&1 < /dev/null &
sleep 45
echo "[swap-b2] $(date +%T) victim restarted on pre-fix binary:"
curl -s -m 5 -X POST http://127.0.0.1:47985 -H 'Content-Type: application/json' \
  -d '{"jsonrpc":"2.0","id":1,"method":"stats.syncInfo","params":[]}' || true
echo
