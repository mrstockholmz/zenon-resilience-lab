#!/usr/bin/env bash
# Runs the Zeno settlement attack checks (phase 26) against the local Zeno
# devnet. Stages the canonical TS from this repo into the zeno-devnet checkout
# (which holds the npm deps and src/devnet helpers) and executes it with tsx.
#
# Prerequisites (see README.md):
#   - Zeno devnet up: cd ~/zeno-local/zeno/zeno-devnet && ZENO_DEVNET_LOCAL=1 scripts/devnet.sh up
#   - executor + gateway containers running
#
# Env:
#   ZENO_DEVNET   path to the zeno-devnet checkout (default ~/zeno-local/zeno/zeno-devnet)
set -euo pipefail

LAB="$(cd "$(dirname "$0")/../.." && pwd)"
PHASE="$LAB/results/phase-26-zeno-attacks"
ZENO_DEVNET="${ZENO_DEVNET:-$HOME/zeno-local/zeno/zeno-devnet}"

if [[ ! -f "$ZENO_DEVNET/src/devnet.ts" ]]; then
  echo "zeno-devnet checkout not found at $ZENO_DEVNET" >&2
  exit 2
fi
if ! curl -sf -m 5 -X POST http://127.0.0.1:55997 -H 'Content-Type: application/json' \
    -d '{"jsonrpc":"2.0","id":1,"method":"ledger.getFrontierMomentum"}' >/dev/null; then
  echo "zeno devnet RPC at 127.0.0.1:55997 does not answer; bring the devnet up first" >&2
  exit 2
fi

cp "$PHASE/zeno-attacks.ts" "$ZENO_DEVNET/flows/zeno-attacks.ts"
mkdir -p "$PHASE/results"
LOG="$PHASE/results/run-$(date +%Y%m%d-%H%M%S).log"
echo "running checks; log: $LOG"
cd "$ZENO_DEVNET"
set +e
ZENO_DEVNET_LOCAL=1 npx tsx flows/zeno-attacks.ts 2>&1 | tee "$LOG"
code="${PIPESTATUS[0]}"
set -e
echo "exit code: $code"
exit "$code"
