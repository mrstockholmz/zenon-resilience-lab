#!/bin/bash
# Zenon Resilience Lab — one-command suite runner
# Usage: ./run-suite.sh [phase2|phase3|all]
#
# Prerequisites:
#   - lab devnet running (znnd on 127.0.0.1:57997)
#   - labctl built (go build -o labctl ./cmd/labctl)
#   - momentum height >= 30 for dynamic plasma tests
#
# Results are written to ../results/phase-N/ as JSON.

set -euo pipefail
cd "$(dirname "$0")"

RESULTS_DIR="$(dirname "$0")/../results"
PHASE2_DIR="$RESULTS_DIR/phase-2"
PHASE3_DIR="$RESULTS_DIR/phase-3"
mkdir -p "$PHASE2_DIR" "$PHASE3_DIR"

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
        # Cases: low-steady-1, low-burst-1, high-steady-1, high-burst-1
        for seed in 0 1 2; do
            for case in low-steady-1 low-burst-1 high-steady-1 high-burst-1; do
                run_phase3_case "$case" "$seed" "$REGIME"
            done
        done
        ;;
    all)
        echo "Running full suite (phase 2 + phase 3)..."
        run_phase2 "$REGIME" r1
        for seed in 0 1 2; do
            for case in low-steady-1 low-burst-1 high-steady-1 high-burst-1; do
                run_phase3_case "$case" "$seed" "$REGIME"
            done
        done
        ;;
    *)
        echo "Usage: $0 [phase2|phase3|all]" >&2
        exit 1
        ;;
esac

echo "=== Suite complete ==="
echo "Results in: $RESULTS_DIR"
