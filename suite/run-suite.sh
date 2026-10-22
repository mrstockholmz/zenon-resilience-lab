#!/bin/bash
# Zenon Resilience Lab — one-command suite runner
# Usage: ./run-suite.sh [phase2|phase3|phase4|phase6|phase7|phase8|phase9|check|all]
#
# Prerequisites:
#   - lab devnet running (znnd on 127.0.0.1:57997)
#   - labctl built (go build -o labctl ./cmd/labctl)
#   - momentum height >= 30 for dynamic plasma tests
#
# Results are written to ../results/phase-N/ as JSON.
# Use ./run-suite.sh check to run regression assertions.

set -euo pipefail
cd "$(dirname "$0")"

RESULTS_DIR="$(dirname "$0")/../results"
PHASE2_DIR="$RESULTS_DIR/phase-2"
PHASE3_DIR="$RESULTS_DIR/phase-3"
PHASE4_DIR="$RESULTS_DIR/phase-4"
PHASE6_DIR="$RESULTS_DIR/phase-6"
PHASE7_DIR="$RESULTS_DIR/phase-7"
PHASE8_DIR="$RESULTS_DIR/phase-8"
PHASE9_DIR="$RESULTS_DIR/phase-9"
mkdir -p "$PHASE2_DIR" "$PHASE3_DIR" "$PHASE4_DIR" "$PHASE6_DIR" "$PHASE7_DIR" "$PHASE8_DIR" "$PHASE9_DIR"

# Check node is reachable
if ! curl -s http://127.0.0.1:57997 -X POST -H 'Content-Type: application/json' \
    -d '{"jsonrpc":"2.0","id":1,"method":"ledger.getFrontierMomentum","params":[]}' > /dev/null 2>&1; then
    echo "ERROR: lab devnet not reachable at 127.0.0.1:57997" >&2
    exit 1
fi

HEIGHT=$(./labctl info | grep momentum_height | awk '{print $2}')
REGIME="static"
if [ "$HEIGHT" -ge 30 ]; then
    REGIME="dynamic"
fi
echo "=== Lab Suite ==="
echo "momentum height: $HEIGHT (regime: $REGIME)"
echo ""

run_phase2() {
    local regime=$1
    local suffix=$2
    
    echo "--- Phase 2: ordinary workload ($regime) ---"
    ./labctl ordinary 3 1 | tee "$PHASE2_DIR/${regime}-ordinary-${suffix}.json"
    echo ""
    
    echo "--- Phase 2: burst workload ($regime) ---"
    ./labctl burst 5 1 | tee "$PHASE2_DIR/${regime}-burst-${suffix}.json"
    echo ""
    
    echo "--- Phase 2: probe ($regime) ---"
    ./labctl probe 30 | tee "$PHASE2_DIR/${regime}-probe-${suffix}.json"
    echo ""
}

run_phase3_case() {
    local case_name=$1
    local seed=$2
    local regime=$3
    
    echo "--- Phase 3: $case_name (seed=$seed, $regime) ---"
    ./labctl matrix "$case_name" "$seed" | tee "$PHASE3_DIR/${case_name}-seed${seed}-${regime}.json"
    echo ""
}

run_phase4_case() {
    local case_name=$1
    local seed=$2
    local regime=$3
    
    echo "--- Phase 4: $case_name (seed=$seed, $regime) ---"
    if [ "$case_name" = "high-steady-1" ]; then
        ./labctl ordinary 5 1 | tee "$PHASE4_DIR/${regime}-high-steady-1-seed${seed}.json"
    elif [ "$case_name" = "high-burst-1" ]; then
        ./labctl burst 5 1 | tee "$PHASE4_DIR/${regime}-high-burst-1-seed${seed}.json"
    fi
    echo ""
}

run_phase6() {
    local seed=$1
    echo "--- Phase 6: hvburst 200 txs (seed=$seed) ---"
    ./labctl hvburst 200 1 2>&1 | tee "$PHASE6_DIR/hvburst-200-dynamic-seed${seed}.json"
    echo ""
    echo "--- Phase 6: waiting 3 momenta for price decay ---"
    ./labctl waitfor $(( $(./labctl info | grep momentum_height | awk '{print $2}') + 3 ))
    echo ""
}

run_phase7() {
    local seed=$1
    echo "--- Phase 7: contend 10 accounts x 30 txs (seed=$seed) ---"
    ./labctl contend 10 30 1 2>&1 | tee "$PHASE7_DIR/contend-10acct-30tx-seed${seed}-v2.json"
    echo ""
    echo "--- Phase 7: waiting 3 momenta for plasma regen ---"
    ./labctl waitfor $(( $(./labctl info | grep momentum_height | awk '{print $2}') + 3 ))
    echo ""
}

run_phase8() {
    local idx=$1
    local seed=$2
    echo "--- Phase 8: deplete account $idx with 100 QSR (seed=$seed) ---"
    ./labctl deplete "$idx" 100 15 2>&1 | tee "$PHASE8_DIR/deplete-acct${idx}-100qsr-seed${seed}.json"
    echo ""
    echo "--- Phase 8: waiting 3 momenta for plasma regen ---"
    ./labctl waitfor $(( $(./labctl info | grep momentum_height | awk '{print $2}') + 3 ))
    echo ""
}

run_phase9() {
    local seed=$1
    echo "--- Phase 9: powlat 14 accounts (seed=$seed) ---"
    ./labctl powlat 14 1 2>&1 | tee "$PHASE9_DIR/powlat-14acct-seed${seed}.json"
    echo ""
}

case "${1:-all}" in
    phase2)
        if [ "$REGIME" = "static" ]; then
            run_phase2 static r1
        else
            run_phase2 dynamic r1
        fi
        ;;
    phase3)
        # Phase 3 matrix: 4 cases x 3 seeds = 12 runs
        for seed in 0 1 2; do
            for case in low-steady-1 low-burst-1 high-steady-1 high-burst-1; do
                run_phase3_case "$case" "$seed" "$REGIME"
            done
        done
        ;;
    phase4)
        # Phase 4: fused plasma comparison across regimes
        # Run 3 seeds of high-steady-1 and high-burst-1 in current regime
        for seed in 0 1 2; do
            run_phase4_case high-steady-1 "$seed" "$REGIME"
            run_phase4_case high-burst-1 "$seed" "$REGIME"
        done
        ;;
    phase6)
        # Phase 6: Dynamic Plasma repricing (3 seeds)
        for seed in 0 1 2; do
            run_phase6 "$seed"
        done
        ;;
    phase7)
        # Phase 7: multi-sender contention (3 seeds)
        for seed in 0 1 2; do
            run_phase7 "$seed"
        done
        ;;
    phase8)
        # Phase 8: plasma depletion + regeneration (3 seeds, accounts 21-23)
        for seed in 0 1 2; do
            run_phase8 $((21 + seed)) "$seed"
        done
        ;;
    phase9)
        # Phase 9: PoW latency (3 seeds)
        for seed in 0 1 2; do
            run_phase9 "$seed"
        done
        ;;
    check)
        echo "Running regression assertions..."
        python3 check-regressions.py "$RESULTS_DIR"
        echo ""
        echo "Exporting CSV..."
        python3 json2csv.py "$RESULTS_DIR" "$RESULTS_DIR/summary.csv"
        ;;
    all)
        echo "Running full suite (phases 2-4)..."
        run_phase2 "$REGIME" r1
        for seed in 0 1 2; do
            for case in low-steady-1 low-burst-1 high-steady-1 high-burst-1; do
                run_phase3_case "$case" "$seed" "$REGIME"
            done
        done
        for seed in 0 1 2; do
            run_phase4_case high-steady-1 "$seed" "$REGIME"
            run_phase4_case high-burst-1 "$seed" "$REGIME"
        done
        echo ""
        echo "Running regression checks..."
        python3 check-regressions.py "$RESULTS_DIR"
        echo ""
        echo "Exporting CSV..."
        python3 json2csv.py "$RESULTS_DIR" "$RESULTS_DIR/summary.csv"
        ;;
    *)
        echo "Usage: $0 [phase2|phase3|phase4|phase6|phase7|phase8|phase9|check|all]" >&2
        exit 1
        ;;
esac

echo "=== Suite complete ==="
echo "Results in: $RESULTS_DIR"
