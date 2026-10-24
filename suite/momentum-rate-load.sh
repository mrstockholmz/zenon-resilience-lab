#!/bin/bash
# Phase 21: Momentum production rate under transaction load.
# Runs the momentum-rate collector while simultaneously sending a transaction burst.
#
# Usage: ./momentum-rate-load.sh [duration_secs] [accounts] [txPerAcct]
# Defaults: 300s duration, 50 accounts, 4 txs each

set -e
cd "$(dirname "$0")/.."

DURATION=${1:-300}
ACCOUNTS=${2:-50}
TXPERACCT=${3:-4}
LABCTL="./suite/labctl"
RESULTS_DIR="results/phase-21"
mkdir -p "$RESULTS_DIR"

SEED=$(date +%s)
OUTFILE="$RESULTS_DIR/momentum-rate-load-${ACCOUNTS}acct-${TXPERACCT}tx-seed${SEED}.json"
TXFILE="$RESULTS_DIR/momentum-rate-load-${ACCOUNTS}acct-${TXPERACCT}tx-seed${SEED}.tx.json"

echo "[momentum-rate-load] Starting load test:"
echo "  Duration: ${DURATION}s"
echo "  Accounts: $ACCOUNTS"
echo "  Txs per account: $TXPERACCT"
echo "  Output: $OUTFILE"
echo ""

# Start momentum rate collector in background
python3 suite/momentum-rate.py --duration "$DURATION" --out "$OUTFILE" &
COLLECTOR_PID=$!

# Give collector a moment to start
sleep 2

# Send transaction burst (saturate: N accounts send M txs each)
echo "[momentum-rate-load] Sending $ACCOUNTS accounts x $TXPERACCT txs..."
START_TS=$(date +%s)
$LABCTL saturate "$ACCOUNTS" "$TXPERACCT" 200 0 2>&1 | tee "$TXFILE"
END_TS=$(date +%s)
echo "[momentum-rate-load] Transactions sent in $((END_TS - START_TS))s"

# Wait for collector to finish
echo "[momentum-rate-load] Waiting for collector (PID $COLLECTOR_PID)..."
wait $COLLECTOR_PID
COLLECTOR_EXIT=$?

echo "[momentum-rate-load] Done (exit=$COLLECTOR_EXIT)"
echo "[momentum-rate-load] Results: $OUTFILE"
echo "[momentum-rate-load] Tx log: $TXFILE"
