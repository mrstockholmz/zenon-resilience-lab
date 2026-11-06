#!/bin/bash
# Zenon Resilience Lab — one-command devnet setup for reproduction.
#
# Creates an isolated solo-pillar devnet from the pinned go-zenon revision:
#   - copies the lab genesis (suite/devnet/genesis.json, committed) into the data dir
#   - copies the disposable public devnet producer keyfile from the go-zenon
#     checkout and patches its embedded "Path" field (the wallet manager
#     indexes keyfiles by that field; a wrong path => ErrKeyStoreNotFound)
#   - writes config.json (127.0.0.1 only, no peers)
#   - starts znnd (if --start is given) and waits for RPC
#
# Usage:
#   scripts/setup-devnet.sh [--start] [--go-zenon PATH] [--data DIR]
#
# Defaults: --go-zenon ../lab-go-zenon (sibling of this repo, as in the README)
#           --data $HOME/lab-devnet
#
# After setup (without --start), run:
#   nice -n 10 <go-zenon>/build/znnd --data <data-dir> &
#   then: cd suite && go build -o labctl ./cmd/labctl && ./labctl info

set -euo pipefail

GO_ZENON="../lab-go-zenon"
DATA_DIR="${HOME}/lab-devnet"
START=0

while [ $# -gt 0 ]; do
    case "$1" in
        --start) START=1; shift ;;
        --go-zenon) GO_ZENON="$2"; shift 2 ;;
        --data) DATA_DIR="$2"; shift 2 ;;
        *) echo "unknown arg: $1" >&2; exit 2 ;;
    esac
done

SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd)"
REPO_DIR="$(dirname "$SCRIPT_DIR")"
GENESIS_SRC="$REPO_DIR/suite/devnet/genesis.json"
KEY_SRC="$GO_ZENON/docker/devnet/pillar/wallet/z1qq9n7fpaqd8lpcljandzmx4xtku9w4ftwyg0mq"
ADDR="z1qq9n7fpaqd8lpcljandzmx4xtku9w4ftwyg0mq"

[ -f "$GENESIS_SRC" ] || { echo "lab genesis not found: $GENESIS_SRC" >&2; exit 1; }
[ -f "$KEY_SRC" ] || { echo "devnet keyfile not found: $KEY_SRC (checkout go-zenon at the pinned revision)" >&2; exit 1; }
[ -x "$GO_ZENON/build/znnd" ] || { echo "znnd not built: cd $GO_ZENON && make znnd" >&2; exit 1; }

mkdir -p "$DATA_DIR/wallet"
cp "$GENESIS_SRC" "$DATA_DIR/genesis.json"
cp "$KEY_SRC" "$DATA_DIR/wallet/$ADDR"

# Patch the keyfile's embedded Path field to its real location; the go-zenon
# wallet manager keys its index by that field, so a stale path makes the
# producer key "not found in the data directory" at node startup.
python3 - "$DATA_DIR" "$ADDR" << 'EOF'
import json, sys
data_dir, addr = sys.argv[1], sys.argv[2]
p = f"{data_dir}/wallet/{addr}"
k = json.load(open(p))
k["Path"] = p
json.dump(k, open(p, "w"), indent=4)
EOF

cat > "$DATA_DIR/config.json" << EOF
{
  "DataPath": "$DATA_DIR",
  "GenesisFile": "$DATA_DIR/genesis.json",
  "LogLevel": "info",
  "Name": "lab-devnet-pillar",
  "Net": {
    "BootstrapPeers": [],
    "ListenHost": "127.0.0.1",
    "ListenPort": 56995,
    "MinConnectedPeers": 0,
    "MinPeers": 0,
    "Seeders": []
  },
  "Producer": {
    "Address": "$ADDR",
    "Index": 0,
    "KeyFilePath": "$ADDR",
    "Password": "devnet"
  },
  "RPC": {
    "EnableHTTP": true,
    "EnableWS": false,
    "Endpoints": [],
    "HTTPCors": [],
    "HTTPHost": "127.0.0.1",
    "HTTPPort": 57997,
    "HTTPVirtualHosts": ["localhost"],
    "WSHost": "127.0.0.1",
    "WSPort": 57998
  }
}
EOF

echo "devnet data dir ready: $DATA_DIR"

if [ "$START" = "1" ]; then
    nohup nice -n 10 "$GO_ZENON/build/znnd" --data "$DATA_DIR" > "$DATA_DIR/znnd.out" 2>&1 &
    echo "znnd started (pid $!), waiting for RPC..."
    for i in $(seq 1 60); do
        if curl -s http://127.0.0.1:57997 -X POST -H 'Content-Type: application/json' \
            -d '{"jsonrpc":"2.0","id":1,"method":"ledger.getFrontierMomentum","params":[]}' \
            | grep -q '"result"'; then
            echo "devnet RPC up at http://127.0.0.1:57997"
            exit 0
        fi
        sleep 2
    done
    echo "ERROR: devnet RPC did not come up; see $DATA_DIR/znnd.out" >&2
    exit 1
fi
