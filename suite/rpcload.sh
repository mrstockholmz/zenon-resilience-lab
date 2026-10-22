#!/bin/bash
# RPC API latency under load — Phase 10
# Measures latency of common RPC endpoints during baseline (no load) and
# under a high-volume transaction burst.
#
# Usage: ./rpcload.sh [num_burst_txs]
#   num_burst_txs: number of transactions in the background burst (default: 200)
#
# Outputs JSON to stdout with per-endpoint latency percentiles.

set -euo pipefail
cd "$(dirname "$0")"

RPC_URL="http://127.0.0.1:57997"
NUM_BURST=${1:-200}
RESULTS_DIR="$(dirname "$0")/../results/phase-10"
mkdir -p "$RESULTS_DIR"

# Endpoints to test (method name + params)
ENDPOINTS=(
    "ledger.getFrontierMomentum|[]"
    "ledger.getFrontierAccountBlock|[\"z1qp3yph55qgresyytz83anynr2f4z39x2z3ej3e\"]"
    "embedded.plasma.get|[\"z1qp3yph55qgresyytz83anynr2f4z39x2z3ej3e\"]"
    "embedded.plasma.getRequiredPoWForAccountBlock|[{\"address\":\"z1qp3yph55qgresyytz83anynr2f4z39x2z3ej3e\",\"blockType\":2}]"
    "stats.get|[{}]"
)

measure_endpoint() {
    local method=$1
    local params=$2
    local n=20
    local latencies=()
    
    for i in $(seq 1 $n); do
        local start end elapsed
        start=$(date +%s%N)
        curl -s "$RPC_URL" -X POST -H 'Content-Type: application/json' \
            -d "{\"jsonrpc\":\"2.0\",\"id\":$i,\"method\":\"$method\",\"params\":$params}" \
            > /dev/null 2>&1
        end=$(date +%s%N)
        elapsed=$(( (end - start) / 1000000 ))  # ms
        latencies+=($elapsed)
    done
    
    # Sort and compute percentiles
    IFS=$'\n' sorted=($(sort -n <<<"${latencies[*]}")); unset IFS
    local p50=${sorted[$((n / 2))]}
    local p90=${sorted[$((n * 9 / 10))]}
    local p99=${sorted[$((n * 99 / 100))]}
    local min=${sorted[0]}
    local max=${sorted[$((n - 1))]}
    
    echo "{\"method\":\"$method\",\"p50_ms\":$p50,\"p90_ms\":$p90,\"p99_ms\":$p99,\"min_ms\":$min,\"max_ms\":$max}"
}

echo "=== RPC Latency Under Load ===" >&2
echo "Burst size: $NUM_BURST txs" >&2

# ---- Baseline (no load) ----
echo "" >&2
echo "--- Baseline (no load) ---" >&2
baseline_results=()
for ep in "${ENDPOINTS[@]}"; do
    IFS='|' read -r method params <<< "$ep"
    echo "  Measuring $method..." >&2
    result=$(measure_endpoint "$method" "$params")
    baseline_results+=("$result")
done

# ---- Start background burst ----
echo "" >&2
echo "--- Starting background burst ($NUM_BURST txs) ---" >&2
./labctl burst "$NUM_BURST" 1 > /dev/null 2>&1 &
burst_pid=$!
sleep 1  # let burst get going

# ---- Under load ----
echo "" >&2
echo "--- Under load (burst running) ---" >&2
load_results=()
for ep in "${ENDPOINTS[@]}"; do
    IFS='|' read -r method params <<< "$ep"
    echo "  Measuring $method..." >&2
    result=$(measure_endpoint "$method" "$params")
    load_results+=("$result")
done

# Wait for burst to finish
wait $burst_pid 2>/dev/null || true

# ---- Output JSON ----
echo "{"
echo "  \"burst_size\": $NUM_BURST,"
echo "  \"baseline\": ["
for i in "${!baseline_results[@]}"; do
    echo "    ${baseline_results[$i]}$([ $i -lt $((${#baseline_results[@]} - 1)) ] && echo ',' || echo '')"
done
echo "  ],"
echo "  \"under_load\": ["
for i in "${!load_results[@]}"; do
    echo "    ${load_results[$i]}$([ $i -lt $((${#load_results[@]} - 1)) ] && echo ',' || echo '')"
done
echo "  ]"
echo "}"
