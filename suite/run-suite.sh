#!/bin/bash
# Zenon Resilience Lab — one-command suite runner
# Usage: ./run-suite.sh [phase2|phase3|phase4|phase6|phase7|phase8|phase9|phase11|phase12|phase13|phase15|phase16|phase17|phase18|phase19|phase20|phase21|phase22|phase24|check|all]
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
PHASE11_DIR="$RESULTS_DIR/phase-11"
PHASE12_DIR="$RESULTS_DIR/phase-12"
PHASE13_DIR="$RESULTS_DIR/phase-13"
PHASE15_DIR="$RESULTS_DIR/phase-15"
PHASE16_DIR="$RESULTS_DIR/phase-16"
PHASE17_DIR="$RESULTS_DIR/phase-17"
PHASE18_DIR="$RESULTS_DIR/phase-18"
PHASE19_DIR="$RESULTS_DIR/phase-19"
PHASE20_DIR="$RESULTS_DIR/phase-20"
PHASE21_DIR="$RESULTS_DIR/phase-21"
PHASE22_DIR="$RESULTS_DIR/phase-22"
PHASE24_DIR="$RESULTS_DIR/phase-24"
mkdir -p "$PHASE2_DIR" "$PHASE3_DIR" "$PHASE4_DIR" "$PHASE6_DIR" "$PHASE7_DIR" "$PHASE8_DIR" "$PHASE9_DIR" "$PHASE11_DIR" "$PHASE12_DIR" "$PHASE13_DIR" "$PHASE15_DIR" "$PHASE16_DIR" "$PHASE17_DIR" "$PHASE18_DIR" "$PHASE19_DIR" "$PHASE20_DIR" "$PHASE21_DIR" "$PHASE22_DIR" "$PHASE24_DIR"

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

run_phase11() {
    local rep=$1
    echo "--- Phase 11: mixcontend 5 fused + 5 unfused (rep=$rep) ---"
    ./labctl mixcontend 5 5 5 1 2>&1 | tee "$PHASE11_DIR/mixcontend-5f-5u-5tx-rep${rep}.json"
    echo ""
}

run_phase12() {
    local config=$1
    echo "--- Phase 12: saturation config $config ---"
    case "$config" in
        A)
            ./labctl saturate 20 15 150 1 2>&1 | tee "$PHASE12_DIR/saturate-20acct-15tx-seed0.json"
            ;;
        B)
            ./labctl saturate 25 20 200 1 2>&1 | tee "$PHASE12_DIR/saturate-25acct-20tx-seed0.json"
            ;;
        C)
            ./labctl saturate 15 30 300 1 2>&1 | tee "$PHASE12_DIR/saturate-15acct-30tx-seed0.json"
            ;;
    esac
    echo ""
    echo "--- Phase 12: waiting 5 momenta for backlog drain ---"
    ./labctl waitfor $(( $(./labctl info | grep momentum_height | awk '{print $2}') + 5 ))
    echo ""
}

run_phase13() {
    local seed=$1
    echo "--- Phase 13: mixed saturation (seed=$seed) ---"
    ./labctl mixsaturate 20 10 10 1 1 2>&1 | tee "$PHASE13_DIR/mixsaturate-20f-10u-10ftx-1utx-seed${seed}.json"
    echo ""
    echo "--- Phase 13: waiting 5 momenta for backlog drain ---"
    ./labctl waitfor $(( $(./labctl info | grep momentum_height | awk '{print $2}') + 5 ))
    echo ""
}

run_phase15() {
    local seed=$1
    local idx=$((41 + seed))
    local qsr=100
    if [ "$seed" -eq 2 ]; then qsr=50; fi
    if [ "$seed" -eq 3 ]; then qsr=10; fi
    echo "--- Phase 15: plasma regeneration (seed=$seed, acct=$idx, ${qsr} QSR) ---"
    ./labctl regen "$idx" "$qsr" 2>"$PHASE15_DIR/regen-acct${idx}-${qsr}qsr-seed${seed}.stderr" | tee "$PHASE15_DIR/regen-acct${idx}-${qsr}qsr-seed${seed}.json"
    echo ""
}

run_phase16() {
    local seed=$1
    local idx=$((52 + seed))
    local qsr=200
    echo "--- Phase 16: transaction type diversity (seed=$seed, acct=$idx, ${qsr} QSR) ---"
    ./labctl txtype "$idx" "$qsr" 2>"$PHASE16_DIR/txtype-acct${idx}-${qsr}qsr-seed${seed}.stderr" | tee "$PHASE16_DIR/txtype-acct${idx}-${qsr}qsr-seed${seed}.json"
    echo ""
}

run_phase17() {
    local seed=$1
    local fc_idx=$((60 + seed))
    local pc_idx=$((70 + seed))
    local fc_qsr=200
    local pc_qsr=8000
    local pc_tx=1200
    echo "--- Phase 17: frontier conflict (seed=$seed, acct=$fc_idx, ${fc_qsr} QSR) ---"
    ./labctl frontconf "$fc_idx" "$fc_qsr" 2>"$PHASE17_DIR/frontconf-acct${fc_idx}-${fc_qsr}qsr-seed${seed}.stderr" | tee "$PHASE17_DIR/frontconf-acct${fc_idx}-${fc_qsr}qsr-seed${seed}.json"
    echo ""
    echo "--- Phase 17: mempool cap (seed=$seed, acct=$pc_idx, ${pc_qsr} QSR, ${pc_tx} txs) ---"
    ./labctl poolcap "$pc_idx" "$pc_qsr" "$pc_tx" 2>"$PHASE17_DIR/poolcap-acct${pc_idx}-${pc_qsr}qsr-${pc_tx}tx-seed${seed}.stderr" | tee "$PHASE17_DIR/poolcap-acct${pc_idx}-${pc_qsr}qsr-${pc_tx}tx-seed${seed}.json"
    echo ""
}

run_phase18() {
    local seed=$1
    local idx=$((80 + seed))
    local qsr=3000
    echo "--- Phase 18: data size limits (seed=$seed, acct=$idx, ${qsr} QSR) ---"
    ./labctl datasize "$idx" "$qsr" 2>"$PHASE18_DIR/datasize-acct${idx}-${qsr}qsr-seed${seed}.stderr" | tee "$PHASE18_DIR/datasize-acct${idx}-${qsr}qsr-seed${seed}.json"
    echo ""
}

run_phase19() {
    local seed=$1
    local base=$((500 + seed * 200))
    echo "--- Phase 19: contract block cap (seed=$seed, base=$base, 100 accounts) ---"
    ./labctl contractcap 100 1 200 "$base" 2>"$PHASE19_DIR/contractcap-seed${seed}.stderr" | tee "$PHASE19_DIR/contractcap-100acct-seed${seed}.json"
    echo ""
}

run_phase20() {
    local seed=$1
    local base=$((400 + seed * 200))
    echo "--- Phase 20: content capacity (seed=$seed, base=$base, 200 self-transfers) ---"
    ./labctl contentmix 200 0 200 "$base" 2>"$PHASE20_DIR/contentmix-A-200self-0stake-seed${seed}.stderr" | tee "$PHASE20_DIR/contentmix-A-200self-0stake-seed${seed}.json"
    echo ""
}

run_phase22() {
    local seed=$1
    local idx=$((80 + seed))
    local qsr=100
    echo "--- Phase 22: validation edge cases (seed=$seed, acct=$idx, ${qsr} QSR) ---"
    ./labctl valedge "$idx" "$qsr" 2>"$PHASE22_DIR/valedge-acct${idx}-${qsr}qsr-seed${seed}.stderr" | tee "$PHASE22_DIR/valedge-acct${idx}-${qsr}qsr-seed${seed}.json"
    echo ""
}

run_phase24() {
    local seed=$1
    local base_idx=$((2000 + seed * 20))
    echo "--- Phase 24: multi-producer PoW latency (seed=$seed, base_idx=$base_idx) ---"
    ./labctl powlat 5 1 "$base_idx" 2>"$PHASE24_DIR/powlat-5acct-3p-seed${seed}.log" | tee "$PHASE24_DIR/powlat-5acct-3p-seed${seed}.json"
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
    phase11)
        # Phase 11: mixed fused/unfused contention (3 repetitions)
        for rep in 0 1 2; do
            run_phase11 "$rep"
        done
        ;;
    phase12)
        # Phase 12: saturation behavior (3 configurations)
        for config in A B C; do
            run_phase12 "$config"
        done
        ;;
    phase13)
        # Phase 13: mixed fused/unfused saturation (3 seeds)
        for seed in 0 1 2; do
            run_phase13 "$seed"
        done
        ;;
    phase15)
        # Phase 15: plasma regeneration rate (4 seeds: 100/100/50/10 QSR)
        for seed in 0 1 2 3; do
            run_phase15 "$seed"
        done
        ;;
    phase16)
        # Phase 16: transaction type diversity (3 seeds)
        for seed in 0 1 2; do
            run_phase16 "$seed"
        done
        ;;
    phase17)
        # Phase 17: mempool cap + frontier conflict (3 seeds)
        for seed in 0 1 2; do
            run_phase17 "$seed"
        done
        ;;
    phase18)
        # Phase 18: data payload size limits (3 seeds)
        for seed in 0 1 2; do
            run_phase18 "$seed"
        done
        ;;
    phase19)
        # Phase 19: contract block throughput cap (3 seeds)
        for seed in 0 1 2; do
            run_phase19 "$seed"
        done
        ;;
    phase20)
        # Phase 20: momentum content capacity (3 seeds)
        for seed in 0 1 2; do
            run_phase20 "$seed"
        done
        ;;
    phase21)
        # Phase 21: momentum production rate analysis
        echo "Phase 21: Momentum production rate analysis"
        echo "  Collecting 5-minute baseline..."
        python3 momentum-rate.py --duration 300 --out "$PHASE21_DIR/momentum-rate-baseline.json"
        echo "  Running burst load test..."
        python3 momentum-rate.py --duration 180 --out "$PHASE21_DIR/momentum-rate-burst.json" &
        COLLECTOR=$!
        sleep 2
        ./labctl burst 100 0 2>&1 | tail -5
        wait $COLLECTOR
        echo "  Phase 21 complete."
        ;;
    phase22)
        # Phase 22: validation edge cases (3 seeds)
        for seed in 0 1 2; do
            run_phase22 "$seed"
        done
        ;;
    phase24)
        # Phase 24: multi-producer PoW latency (3 seeds)
        for seed in 0 1 2; do
            run_phase24 "$seed"
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
        echo "Usage: $0 [phase2|phase3|phase4|phase6|phase7|phase8|phase9|phase11|phase12|phase13|phase15|phase16|phase17|phase18|phase19|phase20|phase21|phase22|phase24|check|all]" >&2
        exit 1
        ;;
esac

echo "=== Suite complete ==="
echo "Results in: $RESULTS_DIR"
